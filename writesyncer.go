package reopen

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

type ReopenableWriteSyncer struct {
	filePath  string
	fileMode  os.FileMode
	reopenSig chan os.Signal
	cur       atomic.Value // *os.File

	closing chan bool
}

func New(file string, mode os.FileMode, sig ...os.Signal) (*ReopenableWriteSyncer, error) {
	ws := &ReopenableWriteSyncer{
		filePath:  file,
		fileMode:  mode,
		reopenSig: make(chan os.Signal, 1),
		closing:   make(chan bool, 1),
	}
	if err := ws.open(); err != nil {
		return nil, err
	}
	if len(sig) == 0 {
		sig = append(sig, syscall.SIGUSR1)
	}
	signal.Notify(ws.reopenSig, sig...)
	go ws.watch()
	return ws, nil
}

func (ws *ReopenableWriteSyncer) Write(p []byte) (n int, err error) {
	return ws.getFile().Write(p)
}

// wrap all the WriteSyncer methods to use getFile
// example with Sync
func (ws *ReopenableWriteSyncer) Sync() error {
	return ws.getFile().Sync()
}

func (ws *ReopenableWriteSyncer) Close() (err error) {
	close(ws.closing)
	_ = ws.getFile().Close()
	return
}

func (ws *ReopenableWriteSyncer) getFile() *os.File {
	return ws.cur.Load().(*os.File)
}

func (ws *ReopenableWriteSyncer) watch() {
	for {
		select {
		case <-ws.closing:
			break
		case <-ws.reopenSig:
			if err := ws.reload(); err != nil {
				break
			}
		}
	}
}

func (ws *ReopenableWriteSyncer) open() error {
	f, err := os.OpenFile(ws.filePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, ws.fileMode)
	if err != nil {
		return err
	}
	ws.cur.Store(f)
	return nil
}

func (ws *ReopenableWriteSyncer) reload() error {
	oldDest := ws.getFile()
	if err := ws.open(); err != nil {
		return err
	}

	go func() {
		time.Sleep(time.Second * 10)
		_ = oldDest.Close()
	}()
	return nil
}
