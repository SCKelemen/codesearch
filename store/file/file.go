package file

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type FlushStrategy int

const (
	FlushImmediate FlushStrategy = iota
	FlushBatched
	FlushManual
)

const defaultBatchWindow = 100 * time.Millisecond

type Option func(*config)

type config struct {
	flushStrategy FlushStrategy
	batchWindow   time.Duration
}

func defaultConfig() config {
	return config{
		flushStrategy: FlushImmediate,
		batchWindow:   defaultBatchWindow,
	}
}

func WithFlushStrategy(strategy FlushStrategy) Option {
	return func(cfg *config) {
		cfg.flushStrategy = strategy
	}
}

func WithBatchWindow(window time.Duration) Option {
	return func(cfg *config) {
		if window > 0 {
			cfg.batchWindow = window
		}
	}
}

type fileLock struct {
	file *os.File
}

func lockFile(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file %q: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("lock %q: %w", path, err)
	}
	return &fileLock{file: file}, nil
}

func (l *fileLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	var errs []error
	if err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN); err != nil {
		errs = append(errs, err)
	}
	if err := l.file.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("create directory %q: %w", path, err)
	}
	return nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := ensureDir(dir); err != nil {
		return err
	}

	tempFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file for %q: %w", path, err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file for %q: %w", path, err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file for %q: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file for %q: %w", path, err)
	}
	if err := os.Chmod(tempPath, perm); err != nil {
		return fmt.Errorf("chmod temp file for %q: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file for %q: %w", path, err)
	}
	return nil
}

func loadGobFile(path string, target any) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("read %q: %w", path, err)
	}
	if len(data) == 0 {
		return true, nil
	}
	decoder := gob.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return true, fmt.Errorf("decode %q: %w", path, err)
	}
	return true, nil
}

func saveGobFile(path string, value any) error {
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encode %q: %w", path, err)
	}
	return atomicWriteFile(path, buffer.Bytes(), 0o644)
}

type persistence struct {
	path         string
	config       config
	lock         *fileLock
	flushMu      sync.Mutex
	timerMu      sync.Mutex
	timer        *time.Timer
	dirty        bool
	closed       bool
	saveSnapshot func() error
}

func newPersistence(dir, filename string, saveSnapshot func() error, options ...Option) (*persistence, error) {
	cfg := defaultConfig()
	for _, option := range options {
		if option != nil {
			option(&cfg)
		}
	}
	if err := ensureDir(dir); err != nil {
		return nil, err
	}
	lock, err := lockFile(filepath.Join(dir, filename+".lock"))
	if err != nil {
		return nil, err
	}
	return &persistence{
		path:         filepath.Join(dir, filename),
		config:       cfg,
		lock:         lock,
		saveSnapshot: saveSnapshot,
	}, nil
}

func (p *persistence) load(target any) (bool, error) {
	return loadGobFile(p.path, target)
}

func (p *persistence) markDirty() error {
	switch p.config.flushStrategy {
	case FlushImmediate:
		return p.Flush()
	case FlushBatched:
		p.timerMu.Lock()
		defer p.timerMu.Unlock()
		if p.closed {
			return nil
		}
		p.dirty = true
		if p.timer == nil {
			p.timer = time.AfterFunc(p.config.batchWindow, func() {
				_ = p.Flush()
			})
			return nil
		}
		p.timer.Reset(p.config.batchWindow)
		return nil
	case FlushManual:
		p.timerMu.Lock()
		p.dirty = true
		p.timerMu.Unlock()
		return nil
	default:
		return fmt.Errorf("unknown flush strategy %d", p.config.flushStrategy)
	}
}

func (p *persistence) Flush() error {
	p.flushMu.Lock()
	defer p.flushMu.Unlock()

	p.timerMu.Lock()
	if p.closed {
		p.timerMu.Unlock()
		return nil
	}
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	shouldWrite := p.dirty || p.config.flushStrategy == FlushImmediate
	p.dirty = false
	p.timerMu.Unlock()

	if !shouldWrite {
		return nil
	}
	return p.saveSnapshot()
}

func (p *persistence) Close() error {
	var errs []error
	if err := p.Flush(); err != nil {
		errs = append(errs, err)
	}
	p.timerMu.Lock()
	p.closed = true
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	p.timerMu.Unlock()
	if err := p.lock.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
