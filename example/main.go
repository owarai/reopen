package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/owarai/reopen"
)

// Note:
// use logrotate to test:
//
// add config file to $HOME/.config/logrotate.conf
// content like this: {{xxx}} aims to be replaced to true value.
// 	{{your_logfile_dir}}/example.log {{your_logfile_dir}}/example.log.wf {
// 	   hourly
// 	   rotate 8
// 	   size 1M
// 	   missingok
// 	   notifempty
// 	   compress
// 	   sharedscripts
// 	   postrotate
// 	   /bin/killall -USR1 {{your_process_name}}
// 	   endscript
// 	}
//
// run this program and run logrotate periodically.
//
// 	logrotate $HOME/.config/logrotate.conf --state $HOME/.config/logrotate-state --verbose
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
		defer cancel()

		select {
		case <-time.After(5 * time.Minute):
			return
		case sig := <-signals:
			fmt.Printf("terminating via signal: %s", sig)
			return
		}
	}()
	zapLog(ctx, "example", time.Millisecond)
}

func zapLog(ctx context.Context, fileBaseName string, writeInterval time.Duration) {
	logger := newLogger(fileBaseName)

	defer logger.Sync()
	rand.Seed(time.Now().Unix())
	url := "www.google.com"
	for {
		time.Sleep(writeInterval)
		select {
		case <-ctx.Done():
			return
		default:
		}
		logger.Info("Operation execution successful", zap.String("url", url),
			zap.Int("attempt", 1), zap.Duration("backoff", 1))
		if rand.Intn(10) >= 5 {
			logger.Error("Failed to fetch URL",
				zap.String("url", url), zap.Int("attempt", 3), zap.Duration("backoff", 1))
		}
	}
}

func newLogger(destName string) *zap.Logger {
	// First, define our level-handling logic.
	highPriority := zap.LevelEnablerFunc(func(lvl zapcore.Level) bool {
		return lvl >= zapcore.ErrorLevel
	})

	// Assume that we have clients for two Kafka topics. The clients implement
	// zapcore.WriteSyncer and are safe for concurrent use. (If they only
	// implement io.Writer, we can use zapcore.AddSync to add a no-op Sync
	// method. If they're not safe for concurrent use, we can add a protecting
	// mutex with zapcore.Lock.)
	logDebugs, _ := reopen.New(destName+".log", 0644)
	logErrors, _ := reopen.New(destName+".log.wf", 0644)

	// Optimize the log output for machine consumption and the console output
	// for human operators.
	const defaultTimeFormat = "2006-01-02 15:04:05.00000"
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		encodeTimeLayout(t, defaultTimeFormat, enc)
	}
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	// Join the outputs, encoders, and level-handling functions into
	// zapcore.Cores, then tee the four cores together.
	core := zapcore.NewTee(
		zapcore.NewCore(encoder, logErrors, highPriority),
		zapcore.NewCore(encoder, logDebugs, zapcore.DebugLevel),
	)

	// From a zapcore.Core, it's easy to construct a Logger.
	logger := zap.New(core)
	return logger
}

// borrow from zapcore.
func encodeTimeLayout(t time.Time, layout string, enc zapcore.PrimitiveArrayEncoder) {
	type appendTimeEncoder interface {
		AppendTimeLayout(time.Time, string)
	}

	if enc, ok := enc.(appendTimeEncoder); ok {
		enc.AppendTimeLayout(t, layout)
		return
	}

	enc.AppendString(t.Format(layout))
}
