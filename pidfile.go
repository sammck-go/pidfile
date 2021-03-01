/*
Package gopidfile provides tools for creating pidfiles and associating them with running processes. It can be used
as a library as part of a daemon, etc., or it can be used standalone as a command line wrapper.
*/
package pidfile

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

// PathCombine works just like filepath.Join() except that any absolute path in the sequence will
// throw away all previous items in the sequence. This allows you to have default directories
// that are overridden.
func PathCombine(pathNames ...string) string {
	result := ""
	for _, pathName := range pathNames {
		if filepath.IsAbs(pathName) {
			result = pathName
		} else {
			result = filepath.Join(result, pathName)
		}
	}
	return result
}

// tryLockFile attempts to place an exclusive lock on a file, without blocking.  If
// successful, (true, nil) is returned. If the lock is already held by someone else, (false, nil)
// is returned.  If an error occurs, (false, err) is returned. Never returns EAGAIN/EWOULDBLOCK.
func tryLockFile(f *os.File) (bool, error) {
	e := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if e != nil {
		if errors.Is(e, syscall.EAGAIN) {
			return false, nil
		}

		return false, e
	}
	return true, nil
}

// lockFileWithContext attempts to clame an flock, retrying periodically until successful or the context
//  is terminated.  If retryTime == 0, only one attempt is made.
func lockFileWithContext(ctx context.Context, f *os.File, retryTime time.Duration) error {
	if retryTime == 0 {
		success, e := tryLockFile(f)
		if e != nil {
			return e
		} else if success {
			return nil
		} else {
			return fmt.Errorf("Lock file %s is in use by another process", f.Name())
		}
	} else {
		retryTimer := time.NewTimer(retryTime)
		defer retryTimer.Stop()
		for {
			success, e := tryLockFile(f)
			if e != nil {
				return e
			}
			if success {
				return nil
			}

			select {
			case <-retryTimer.C:
				retryTimer.Reset(retryTime)

			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// unlockFile releases an exclusive file lock that was claimed with tryLocKFile.
func unlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

// inodeOfFile returns the inode associated with an open file.
func inodeOfFile(f *os.File) (uint64, error) {
	var stat syscall.Stat_t
	e := syscall.Fstat(int(f.Fd()), &stat)
	if e != nil {
		return 0, e
	}
	return stat.Ino, nil
}

// inodeOfPathName returns the inode associated with a pathname.
func inodeOfPathName(pathName string) (uint64, error) {
	var stat syscall.Stat_t
	e := syscall.Stat(pathName, &stat)
	if e != nil {
		return 0, e
	}
	return stat.Ino, nil
}

// Config provides configuration options for contruction of a PidFile.  The constructed object is immutable
// after it is constructed by NewConfig.
type Config struct {
	// dirName is the name of the directory of the pidfile. If omitted, /run is used. If a relative path,
	// relative to the current workign directory.
	dirName string

	// fileName is the filename for the pidfile. If relative, it is resolved relative
	// to Dirname. If omitted, <progname>.pid is used.
	fileName string

	// createDir enables creation of the directory containing the pidfile if it does not exist.
	createDir bool

	// createDirMode sets the mode bits that should be used for newly created directories
	// that will contain the pidfile.  Defaults to 0755 (read/write for owner, read for others).
	createDirMode os.FileMode

	// fileMode sets the mode bits that will be used for the pidfile. Defaults to 0755.
	fileMode os.FileMode

	// UseFlock can be set to true to enable use of a parallel lockfile that guarantees
	// atomicity of acquisition of ownership of the pidfile. If disabled, there is
	// a small chance that two copies of the program launched at the same time may both
	// believe they own the pidfile, but only the last instance launched will have its pid
	// in the file associated with the directory entry; the other will be holding a deleted
	// file.
	useFlock bool

	// acquireTimeout is the maximum time to wait for exclusive control of the pidfile. By default,
	// an error will be returned immediately if the pidfile is owned
	acquireTimeout time.Duration

	// retryInterval is the time between retries to gain exclusive control of the pidfile. By default,
	// 500ms is used
	retryInterval time.Duration

	// procPid is the pid to use for the process. If omitted, the pid of the current process is used.
	procPid int

	// deferPublish defers the last step of activation--writing the PID number and renaming the temporary
	// file onto the final pid file until Publish() or PublishWithPid() is called.  This allows exclusive
	// ownership of the pidfile to be claimed before the pid is known or before the daemon is
	// ready for use. Disabled by default.
	deferPublish bool
}

// ConfigOption is an opaque configuration option setter created by one of the With functions.
// It follows the Golang "options" pattern.
type ConfigOption func(*Config)

const (
	defaultDirName        = "/run"
	defaultFileName       = ""
	defaultCreateDir      = false
	defaultCreateDirMode  = os.FileMode(0755)
	defaultFileMode       = os.FileMode(0644)
	defaultUseFlock       = true
	defaultAcquireTimeout = time.Duration(0)
	defaultRetryInterval  = time.Millisecond * 500
	defaultDeferPublish   = false
)

// NewConfig creates a PidFile Config object from provided options. The resulting object
// can be passed to NewPidFile using WithConfig.
func NewConfig(opts ...ConfigOption) *Config {
	cfg := &Config{
		dirName:        defaultDirName,
		fileName:       defaultFileName,
		createDir:      defaultCreateDir,
		createDirMode:  defaultCreateDirMode,
		fileMode:       defaultFileMode,
		useFlock:       defaultUseFlock,
		acquireTimeout: defaultAcquireTimeout,
		retryInterval:  defaultRetryInterval,
		procPid:        0,
		deferPublish:   defaultDeferPublish,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}

// WithConfig allows initialization of a new configuration object starting with an existing one,
// and incremental initialization of configuration separately from initialization of the PidFile.
// If provided, this option should be appear first in the option list, since it replaces all
// configuration values.
func WithConfig(other *Config) ConfigOption {
	return func(cfg *Config) {
		cfg.dirName = other.dirName
		cfg.fileName = other.fileName
		cfg.createDir = other.createDir
		cfg.createDirMode = other.createDirMode
		cfg.fileMode = other.fileMode
		cfg.useFlock = other.useFlock
		cfg.acquireTimeout = other.acquireTimeout
		cfg.retryInterval = other.retryInterval
		cfg.procPid = other.procPid
		cfg.deferPublish = other.deferPublish
	}
}

// Refine creates a new Config object by applying ConfigOptions to an existing config.
func (cfg *Config) Refine(opts ...ConfigOption) *Config {
	newOpts := append([]ConfigOption{WithConfig(cfg)}, opts...)
	newConfig := NewConfig(newOpts...)
	return newConfig
}

// WithDirName sets the name of the directory of the pidfile. If omitted, /run is used. If a relative path,
// it is relative to the current working directory wne the pidfile is created.
func WithDirName(dirName string) ConfigOption {
	return func(cfg *Config) {
		cfg.dirName = dirName
	}
}

// WithFileName is the filename for the pidfile. If relative, it is resolved relative
// to DirName. If omitted, <progname>.pid is used.
func WithFileName(fileName string) ConfigOption {
	return func(cfg *Config) {
		cfg.fileName = fileName
	}
}

// WithCreateDir enables creation of the directory, and parent directories, containing
// the pidfile if they do not exist. By default, an error will occur if the directory does not exist.
func WithCreateDir() ConfigOption {
	return func(cfg *Config) {
		cfg.createDir = true
	}
}

// WithoutCreateDir disables creation of the directory, and parent directories, containing
// the pidfile if they do not exist. This is the default.
func WithoutCreateDir() ConfigOption {
	return func(cfg *Config) {
		cfg.createDir = false
	}
}

// WithCreateDirMode sets the mode bits that should be used for newly created directories
// that will contain the pidfile.  Defaults to 0755 (read/write for owner, read for others).
// This option also implicitly enables WithCreateDir.
func WithCreateDirMode(dirMode os.FileMode) ConfigOption {
	return func(cfg *Config) {
		cfg.createDirMode = dirMode
		cfg.createDir = true
	}
}

// WithFileMode sets the mode bits that will be used for the pidfile. Defaults to 0755.
func WithFileMode(fileMode os.FileMode) ConfigOption {
	return func(cfg *Config) {
		cfg.fileMode = fileMode
	}
}

// WithFlock enables use of a parallel lockfile that guarantees atomicity and mutual exclusion of acquisition
// of ownership of the pidfile. If disabled, there is a small chance that two copies
// of the program launched at the same time may both believe they own the pidfile, but
// only the last instance launched will have its pid in the file associated with the
// directory entry; the other will be holding a deleted file. This file is not
// deleted when the pidfile is closed, but is safe to place on a tmpfs. This is the default setting.
func WithFlock() ConfigOption {
	return func(cfg *Config) {
		cfg.useFlock = true
	}
}

// WithoutFlock disables use of a parallel lockfile that guarantees atomicity and mutual exclusion of acquisition
// of ownership of the pidfile. There will be a small chance that two copies
// of the program launched at the same time may both believe they own the pidfile, but
// only the last instance launched will have its pid in the file associated with the
// directory entry; the other will be holding a deleted file. By default, flock is enabled.
func WithoutFlock() ConfigOption {
	return func(cfg *Config) {
		cfg.useFlock = false
	}
}

// WithAcquireTimeout sets the maximum time to wait for exclusive control of the pidfile. By default,
// an error will be returned immediately if the pidfile cannot be claimed immediately.
func WithAcquireTimeout(acquireTimeout time.Duration) ConfigOption {
	return func(cfg *Config) {
		cfg.acquireTimeout = acquireTimeout
	}
}

// WithRetryInterval sets the time between retries to gain exclusive control of the pidfile. By default,
// 500ms is used.
func WithRetryInterval(retryInterval time.Duration) ConfigOption {
	return func(cfg *Config) {
		cfg.retryInterval = retryInterval
	}
}

// WithPid sets the PID that will be placed in the pidfile. By default, the PID of the current
// process is used.
func WithPid(pid int) ConfigOption {
	return func(cfg *Config) {
		cfg.procPid = pid
	}
}

// WithDeferPublish defers the last step of activation--writing the PID number and renaming the temporary
// file onto the final pid file until Publish() or PublishWithPid() is called.  This allows exclusive
// ownership of the pidfile to be claimed before the pid is known or before the daemon is
// ready for use. Disabled by default.
func WithDeferPublish() ConfigOption {
	return func(cfg *Config) {
		cfg.deferPublish = true
	}
}

// WithoutDeferPublish disabled deferal of the last step of activation--writing the PID number and
// renaming the temporary file onto the final pid file. This is the default setting.
func WithoutDeferPublish() ConfigOption {
	return func(cfg *Config) {
		cfg.deferPublish = false
	}
}

// PidFile is a state machine that manages the lifecycle of a pidfile through locking, publishing,
// destruction and unlocking.
type PidFile struct {
	// lock is a general-purpose fine-grained mutex for the pidfile.
	lock sync.Mutex

	// cfg contains the configuration supplied when the pidfile was created
	cfg *Config

	// dirName contains the resolved absolute path of the directory containing the pidfile
	dirName string

	// pathName contains the resolved absolute path of the pidfile
	pathName string

	// fileName contains the base filename pidfile
	fileName string

	// flockPathName contains the resolved absolute path of the flock file
	flockPathName string

	// errorState is set to an error when a persistent error occurs
	errorState error

	// activateIsStarted is true if activation has begun
	activateIsStarted bool

	// activateDone is true if the pidfile is activated
	activateDone bool

	// activateDoneChan is a channel that is closed after activation is complete
	// or an error occurs
	activateDoneChan chan struct{}

	// publishIsStarted is true if publishing has begun
	publishIsStarted bool

	// publishDone is true if the publish step has completed, possibly unsuccessfully
	publishDone bool

	// isPublished is true if the pidfile has been created successfully
	isPublished bool

	// publishDoneChan is a channel that is closed after publishing is complete
	// or an error occurs
	publishDoneChan chan struct{}

	// closeIsStarted is true if closing has begun
	closeIsStarted bool

	// closeStartedChan is a channel that is closed when closing is initiated
	// or an error occurs
	closeStartedChan chan struct{}

	// isTrue is true if the pidfile is closed
	closeDone bool

	// closeDoneChan is a channel that is closed after close is complete
	// or an error occurs
	closeDoneChan chan struct{}

	// flockFd is the open file used to hold the file lock
	flockFd *os.File

	// fd is the open file descriptor for the pidfile
	fd *os.File

	// inode is the inode identifier of the pidfile when we created it. Used to
	// minimize chances of deleting a pidfile that was replaced after we created it.
	inode uint64

	// publishedPid is the actual PID that is published; it may differ from cfg.procPid
	// if PublishWithPid is used.
	publishedPid int

	// tempPathName is the temporary pathname of the pidfile before it is published
	tempPathName string
}

func (pf *PidFile) GetPid() int {
	pf.lock.Lock()
	defer pf.lock.Unlock()

	return pf.publishedPid
}

func (pf *PidFile) GetPathName() string {
	// lock unnecessary since immutable after activation
	return pf.pathName
}

// startShutdownLocked begins shutting down a PidFile if it has not already started shutting down.
// Does not wait for complete shutdown. If not nil, completionErr provides a reason for the
// shutdown that will be returned from subsequent API calls. Safe to call multiple times; only
// the first call is effective. This fuction *MUST* be called with pf.lock held.
func (pf *PidFile) startShutdownLocked(completionErr error) {
	if !pf.closeIsStarted {
		if pf.errorState == nil {
			pf.errorState = completionErr
		}
		pf.closeIsStarted = true
		close(pf.closeStartedChan)
		go func() {
			deletePidFile := false
			pf.lock.Lock()
			defer pf.lock.Unlock()

			if pf.activateIsStarted {
				pf.lock.Unlock()
				// Because we have closed closeStartedChan, this will complete quickly
				select {
				case <-pf.activateDoneChan:
				}
				pf.lock.Lock()
			} else {
				pf.activateIsStarted = true
				pf.activateDone = true
				close(pf.activateDoneChan)
			}

			if pf.publishIsStarted {
				pf.lock.Unlock()
				// Because we have closed closeStartedChan, this will complete quickly
				select {
				case <-pf.publishDoneChan:
				}
				pf.lock.Lock()
				deletePidFile = pf.isPublished
			} else {
				pf.publishIsStarted = true
				pf.publishDone = true
				close(pf.publishDoneChan)
			}

			if pf.fd != nil {
				_ = pf.fd.Close()
				pf.fd = nil
			}

			if pf.tempPathName != "" {
				_ = os.Remove(pf.tempPathName)
				pf.tempPathName = ""
			}

			if deletePidFile {
				pf.isPublished = false
				inode, e := inodeOfPathName(pf.pathName)
				if e == nil && inode == pf.inode {
					_ = os.Remove(pf.pathName)
				}
			}

			if pf.flockFd != nil {
				_ = unlockFile(pf.flockFd)
				_ = pf.flockFd.Close()
				pf.flockFd = nil
			}

			pf.closeDone = true
			close(pf.closeDoneChan)
		}()
	}
}

// StartShutdown begins shutting down a PidFile if it has not already started shutting down.
// Does not wait for complete shutdown. If not nil, completionErr provides a reason for the
// shutdown that will be returned from subsequent API calls. Safe to call multiple times; only
// the first call is effective.
func (pf *PidFile) StartShutdown(completionErr error) {
	pf.lock.Lock()
	defer pf.lock.Unlock()

	pf.startShutdownLocked(completionErr)
}

// WaitForShutdownWithContext waits for a PidFile to completely shut down complete shutdown.
// It does not initiate shutdown itself. ctx allows the caller to abandon waiting for completion.
// The returned error is the final Close() return value, or the context error.
func (pf *PidFile) WaitForShutdownWithContext(ctx context.Context) error {
	select {
	case <-pf.closeDoneChan:
		pf.lock.Lock()
		defer pf.lock.Unlock()
		return pf.errorState
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitForShutdown waits for a PidFile to completely shut down complete shutdown.
// It does not initiate shutdown itself.
// The returned error is the final Close() return value, or the context error.
func (pf *PidFile) WaitForShutdown() error {
	return pf.WaitForShutdownWithContext(context.Background())
}

// ShutdownWithContext shuts down a PidFile and waits for complete shutdown. If not nil,
// completionErr provides a reason for the shutdown that will be returned from subsequent
// API calls. Safe to call multiple times; only the first call's completionErr is effective.
// ctx allows the caller to abandon waiting for completion, though once started, the pidfile
// will continue shutting down.
func (pf *PidFile) ShutdownWithContext(ctx context.Context, completionErr error) error {
	pf.StartShutdown(completionErr)
	return pf.WaitForShutdownWithContext(ctx)
}

// Shutdown shuts down a PidFile and waits for complete shutdown. If not nil,
// completionErr provides a reason for the shutdown that will be returned from subsequent
// API calls. Safe to call multiple times; only the first call's completionErr is effective.
func (pf *PidFile) Shutdown(completionErr error) error {
	return pf.ShutdownWithContext(context.Background(), completionErr)
}

// CloseWithContext shuts down and deletes the PidFile, and waits for completion. ctx
// allows the caller to abandon waiting for completion, though once started, the pidfile
// will continue shutting down.
func (pf *PidFile) CloseWithContext(ctx context.Context) error {
	return pf.ShutdownWithContext(ctx, nil)
}

// Close shuts down and deletes the PidFile, and waits for completion. Safe to call multiple
// times.
func (pf *PidFile) Close() error {
	return pf.Shutdown(nil)
}

// finalize is called in case a PidFile object is gc'd without closing it. It improves the odds
// that the pidfile is deleted in this case. However, it should not be depended upon; the calling
// app should close() the pidfile before exiting.
func finalize(pf *PidFile) {
	// locking is not necessary at this point
	if !pf.closeDone {
		fd := pf.fd
		pf.fd = nil
		flockFd := pf.flockFd
		pf.flockFd = nil
		inode := pf.inode
		pathName := pf.pathName
		tempPathName := pf.tempPathName
		activateDone := pf.activateDone
		activateDoneChan := pf.activateDoneChan
		publishDone := pf.publishDone
		publishDoneChan := pf.publishDoneChan
		isPublished := pf.isPublished
		closeIsStarted := pf.closeIsStarted
		closeStartedChan := pf.closeStartedChan
		closeDoneChan := pf.closeDoneChan

		// we do these synchronously to improve chances of completion prior to process exit.
		if tempPathName != "" {
			os.Remove(tempPathName)
		}
		if isPublished {
			currentInode, e := inodeOfPathName(pathName)
			if e == nil && currentInode == inode {
				_ = os.Remove(pathName)
			}
		}

		go func() {
			if !closeIsStarted {
				close(closeStartedChan)
			}
			if !activateDone {
				close(activateDoneChan)
			}
			if !publishDone {
				close(publishDoneChan)
			}
			if fd != nil {
				fd.Close()
			}
			if flockFd != nil {
				_ = unlockFile(flockFd)
				_ = flockFd.Close()
			}
			close(closeDoneChan)
		}()
	}
}

// ActivateWithContext does as much as possible to prepare for publishing a pidfile as possible
// without knowing the pid or actually publishing. This includes creating the pidfile
// directory, claiming an exclusive lock (if enabled), and creating the temporary pidfile
// that will be renamed over the pidfile at publishing time.  ctx provides a way for the
// caller to abandon waiting for completion.
func (pf *PidFile) ActivateWithContext(ctx context.Context) error {
	pf.lock.Lock()
	isLocked := true
	var e error
	needActivateDone := false

	cleanup := func() {
		needCloseDone := false

		if !isLocked {
			pf.lock.Lock()
		}

		if needActivateDone {
			if !pf.activateDone {
				if e == nil {
					e = fmt.Errorf("PidFile activation failed")
				}
				pf.activateIsStarted = true
				pf.startShutdownLocked(e)
				needCloseDone = true
				pf.activateDone = true
				close(pf.activateDoneChan)
			}
		}

		pf.lock.Unlock()

		if needCloseDone {
			_ = pf.WaitForShutdownWithContext(ctx)
		}
	}

	defer cleanup()

	if pf.errorState != nil {
		return pf.errorState
	}

	if pf.cfg.acquireTimeout != 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, pf.cfg.acquireTimeout)
		defer cancel()
	}

	if pf.activateIsStarted {
		pf.lock.Unlock()
		isLocked = false
		select {
		case <-pf.activateDoneChan:
			pf.lock.Lock()
			isLocked = true
			if pf.errorState != nil {
				return pf.errorState
			}
			if pf.closeIsStarted {
				return fmt.Errorf("PidFile is closed: %s", pf.pathName)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	pf.activateIsStarted = true
	needActivateDone = true

	pf.lock.Unlock()
	isLocked = false

	if pf.cfg.createDir {
		e = os.MkdirAll(pf.dirName, pf.cfg.createDirMode)
		if e != nil {
			return e
		}
	}

	if pf.cfg.useFlock {
		pf.flockFd, e = os.OpenFile(pf.flockPathName, os.O_RDWR|os.O_CREATE, pf.cfg.fileMode)
		if e != nil {
			return e
		}

		retryInterval := pf.cfg.retryInterval
		if pf.cfg.acquireTimeout == 0 {
			retryInterval = 0
		}

		e = lockFileWithContext(ctx, pf.flockFd, retryInterval)
		if e != nil {
			return e
		}
	}

	pf.fd, e = ioutil.TempFile(pf.dirName, pf.fileName)
	if e != nil {
		return e
	}
	pf.tempPathName = pf.fd.Name()

	pf.inode, e = inodeOfFile(pf.fd)
	if e != nil {
		return e
	}

	e = os.Chmod(pf.tempPathName, pf.cfg.fileMode)
	if e != nil {
		return e
	}

	pf.lock.Lock()
	isLocked = true

	pf.activateDone = true
	close(pf.activateDoneChan)

	needActivateDone = false

	return nil
}

// Activate does as much as possible to prepare for publishing a pidfile as possible
// without knowing the pid or actually publishing. This includes creating the pidfile
// directory, claiming an exclusive lock (if enabled), and creating the temporary pidfile
// that will be renamed over the pidfile at publishing time.
func (pf *PidFile) Activate() error {
	return pf.ActivateWithContext(context.Background())
}

// PublishWithPidAndContext finalizes publication of the pidfile with a provided PID. Activation is also
// performed if necessary. ctx provides a way for the
// caller to abandon waiting for completion.
func (pf *PidFile) PublishWithPidAndContext(ctx context.Context, pid int) error {
	e := pf.ActivateWithContext(ctx)
	if e != nil {
		return e
	}
	pf.lock.Lock()
	isLocked := true
	needPublishDone := false

	cleanup := func() {
		needCloseDone := false

		if !isLocked {
			pf.lock.Lock()
		}

		if needPublishDone {
			if !pf.publishDone {
				if e == nil {
					e = fmt.Errorf("PidFile publish failed")
				}
				pf.publishIsStarted = true
				pf.startShutdownLocked(e)
				needCloseDone = true
				pf.publishDone = true
				close(pf.publishDoneChan)
			}
		}

		pf.lock.Unlock()

		if needCloseDone {
			_ = pf.WaitForShutdownWithContext(ctx)
		}
	}

	defer cleanup()

	if pf.publishIsStarted {
		pf.lock.Unlock()
		isLocked = false
		select {
		case <-pf.publishDoneChan:
			pf.lock.Lock()
			isLocked = true
			if pf.errorState != nil {
				return pf.errorState
			}
			if pf.closeIsStarted {
				return fmt.Errorf("PidFile is closed: %s", pf.pathName)
			}
			if pid != pf.publishedPid {
				return fmt.Errorf("PidFile already published with conflicting pid %d: %s", pf.publishedPid, pf.pathName)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	pf.publishIsStarted = true
	needPublishDone = true

	pf.publishedPid = pid

	_, e = fmt.Fprintf(pf.fd, "%d", pid)
	if e != nil {
		return e
	}

	e = os.Rename(pf.tempPathName, pf.pathName)
	if e != nil {
		return e
	}
	pf.isPublished = true
	pf.tempPathName = ""
	pf.publishDone = true
	close(pf.publishDoneChan)

	needPublishDone = false

	return nil
}

// PublishWithContext finalizes publication of the pidfile with the PID determined at configuration
// time. Activation is also performed if necessary. ctx provides a way for the
// caller to abandon waiting for completion.
func (pf *PidFile) PublishWithContext(ctx context.Context) error {
	return pf.PublishWithPidAndContext(ctx, pf.cfg.procPid)
}

// PublishWithPid finalizes publication of the pidfile with a provided PID. Activation is also
// performed if necessary.
func (pf *PidFile) PublishWithPid(pid int) error {
	return pf.PublishWithPidAndContext(context.Background(), pid)
}

// Publish finalizes publication of the pidfile with the PID determined at configuration
// time. Activation is also performed if necessary.
func (pf *PidFile) Publish() error {
	return pf.PublishWithContext(context.Background())
}

// NewPidFileWithContext activates and optionally publishes a pidfile from a set of configuration options, and allows
// a context to be provided for cancellation, etc.
func NewPidFileWithContext(ctx context.Context, opts ...ConfigOption) (*PidFile, error) {
	cfg := NewConfig(opts...)

	if cfg.fileName == "" {
		progPath, e := os.Executable()
		if e != nil {
			return nil, e
		}
		cfg.fileName = filepath.Base(progPath) + ".pid"
	}

	if cfg.procPid == 0 {
		cfg.procPid = os.Getpid()
	}

	wd, e := os.Getwd()
	if e != nil {
		return nil, e
	}

	pathName := PathCombine(wd, cfg.dirName, cfg.fileName)
	fileName := filepath.Base(pathName)
	dirName := filepath.Dir(pathName)
	flockPathName := pathName + ".lock"

	pf := &PidFile{
		lock:              sync.Mutex{},
		cfg:               cfg,
		dirName:           dirName,
		pathName:          pathName,
		fileName:          fileName,
		flockPathName:     flockPathName,
		errorState:        nil,
		activateIsStarted: false,
		activateDone:      false,
		activateDoneChan:  make(chan struct{}),
		publishIsStarted:  false,
		publishDone:       false,
		isPublished:       false,
		publishDoneChan:   make(chan struct{}),
		closeIsStarted:    false,
		closeStartedChan:  make(chan struct{}),
		closeDone:         false,
		closeDoneChan:     make(chan struct{}),
		flockFd:           nil,
		fd:                nil,
		inode:             0,
		publishedPid:      cfg.procPid,
		tempPathName:      "",
	}

	// For completeness, if pidfile object is gc'd without Closing, we will clean up
	runtime.SetFinalizer(pf, finalize)

	err := pf.ActivateWithContext(ctx)
	if err != nil {
		return nil, err
	}

	if !pf.cfg.deferPublish {
		err = pf.PublishWithContext(ctx)
		if err != nil {
			return nil, err
		}
	}

	return pf, nil
}

// NewPidFile activates and optionally publishes a pidfile from a set of configuration options.
func NewPidFile(opts ...ConfigOption) (*PidFile, error) {
	return NewPidFileWithContext(context.Background(), opts...)
}
