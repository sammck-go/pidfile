# pidfile

pidfile is a simple package for managing pidfiles associated with running processes.

### Features

- Easy to use with sensible defaults and golang option pattern
- Guaranteed atomicity of update of pidfile contents
- File locking ensures only one process associated with a pidfile can be running at a given time
- Ability to associatae the pid with another process; e.g., a spawned child
- Optional separation of setup and lock claiming from publishing of the pidfile, allowing the pid of
  a child process to be determined after the lock is claimed
- Optional timeout/retry interval for claiming lock ownership
- support for context/cancellation
- A best effort is made to delete pidfiles when processes terminate
- A command-line wrapper is included in cmd/with-pidfile that allows you to run any shell command wrapped with a pidfile

### Install

**Binaries**

[![Releases](https://img.shields.io/github/release/sammck-go/pidfile.svg)](https://github.com/sammck-go/pidfile/releases) [![Releases](https://img.shields.io/github/downloads/sammck-go/pidfile/total.svg)](https://github.com/sammck-go/pidfile/releases)

See [the latest release](https://github.com/sammck-go/pidfile/releases/latest)


**Source**

```sh
$ go get -v github.com/sammck-go/pidfile
```


### Commandline Usage

<!-- render these help texts by hand,
  or use https://github.com/jpillora/md-tmpl
    with $ md-tmpl -w README.md -->

<!--tmpl,code=plain:echo "$ with-pidfile --help" && go run cmd/with-pidfile/with-pidfile.go --help -->
``` plain 
$ with-pidfile --help
Usage: with-pidfile [<option>...] <cmd> [<cmd-arg>...]

Wrap a process invocation with a pidfile.

   <cmd>          A program to launch
   <cmd-arg>...   Zero or more arguments or options to the launched program

Options:
  -d string
    	Directory for pidfile. Defaults to /run.
  -f string
    	pidfile name. Defaults to <prog>.pid
  -t int
    	Maximum seconds to wait for pifdile lock. Defaults to 0.
```
<!--/tmpl-->

### Package Usage


<!--tmpl:echo && godocdown -template ./.godocdown.template -->
```go
import "github.com/sammck-go/pidfile"
```

Package gopidfile provides tools for creating pidfiles and associating them with
running processes. It can be used as a library as part of a daemon, etc., or it
can be used standalone as a command line wrapper.

## Usage

#### func  PathCombine

```go
func PathCombine(pathNames ...string) string
```
PathCombine works just like filepath.Join() except that any absolute path in the
sequence will throw away all previous items in the sequence. This allows you to
have default directories that are overridden.

#### type Config

```go
type Config struct {
}
```

Config provides configuration options for contruction of a PidFile. The
constructed object is immutable after it is constructed by NewConfig.

#### func  NewConfig

```go
func NewConfig(opts ...ConfigOption) *Config
```
NewConfig creates a PidFile Config object from provided options. The resulting
object can be passed to NewPidFile using WithConfig.

#### func (*Config) Refine

```go
func (cfg *Config) Refine(opts ...ConfigOption) *Config
```
Refine creates a new Config object by applying ConfigOptions to an existing
config.

#### type ConfigOption

```go
type ConfigOption func(*Config)
```

ConfigOption is an opaque configuration option setter created by one of the With
functions. It follows the Golang "options" pattern.

#### func  WithAcquireTimeout

```go
func WithAcquireTimeout(acquireTimeout time.Duration) ConfigOption
```
WithAcquireTimeout sets the maximum time to wait for exclusive control of the
pidfile. By default, an error will be returned immediately if the pidfile cannot
be claimed immediately.

#### func  WithConfig

```go
func WithConfig(other *Config) ConfigOption
```
WithConfig allows initialization of a new configuration object starting with an
existing one, and incremental initialization of configuration separately from
initialization of the PidFile. If provided, this option should be appear first
in the option list, since it replaces all configuration values.

#### func  WithCreateDir

```go
func WithCreateDir() ConfigOption
```
WithCreateDir enables creation of the directory, and parent directories,
containing the pidfile if they do not exist. By default, an error will occur if
the directory does not exist.

#### func  WithCreateDirMode

```go
func WithCreateDirMode(dirMode os.FileMode) ConfigOption
```
WithCreateDirMode sets the mode bits that should be used for newly created
directories that will contain the pidfile. Defaults to 0755 (read/write for
owner, read for others). This option also implicitly enables WithCreateDir.

#### func  WithDeferPublish

```go
func WithDeferPublish() ConfigOption
```
WithDeferPublish defers the last step of activation--writing the PID number and
renaming the temporary file onto the final pid file until Publish() or
PublishWithPid() is called. This allows exclusive ownership of the pidfile to be
claimed before the pid is known or before the daemon is ready for use. Disabled
by default.

#### func  WithDirName

```go
func WithDirName(dirName string) ConfigOption
```
WithDirName sets the name of the directory of the pidfile. If omitted, /run is
used. If a relative path, it is relative to the current working directory wne
the pidfile is created.

#### func  WithFileMode

```go
func WithFileMode(fileMode os.FileMode) ConfigOption
```
WithFileMode sets the mode bits that will be used for the pidfile. Defaults to
0755.

#### func  WithFileName

```go
func WithFileName(fileName string) ConfigOption
```
WithFileName is the filename for the pidfile. If relative, it is resolved
relative to DirName. If omitted, <progname>.pid is used.

#### func  WithFlock

```go
func WithFlock() ConfigOption
```
WithFlock enables use of a parallel lockfile that guarantees atomicity and
mutual exclusion of acquisition of ownership of the pidfile. If disabled, there
is a small chance that two copies of the program launched at the same time may
both believe they own the pidfile, but only the last instance launched will have
its pid in the file associated with the directory entry; the other will be
holding a deleted file. This file is not deleted when the pidfile is closed, but
is safe to place on a tmpfs. This is the default setting.

#### func  WithPid

```go
func WithPid(pid int) ConfigOption
```
WithPid sets the PID that will be placed in the pidfile. By default, the PID of
the current process is used.

#### func  WithRetryInterval

```go
func WithRetryInterval(retryInterval time.Duration) ConfigOption
```
WithRetryInterval sets the time between retries to gain exclusive control of the
pidfile. By default, 500ms is used.

#### func  WithoutCreateDir

```go
func WithoutCreateDir() ConfigOption
```
WithoutCreateDir disables creation of the directory, and parent directories,
containing the pidfile if they do not exist. This is the default.

#### func  WithoutDeferPublish

```go
func WithoutDeferPublish() ConfigOption
```
WithoutDeferPublish disabled deferal of the last step of activation--writing the
PID number and renaming the temporary file onto the final pid file. This is the
default setting.

#### func  WithoutFlock

```go
func WithoutFlock() ConfigOption
```
WithoutFlock disables use of a parallel lockfile that guarantees atomicity and
mutual exclusion of acquisition of ownership of the pidfile. There will be a
small chance that two copies of the program launched at the same time may both
believe they own the pidfile, but only the last instance launched will have its
pid in the file associated with the directory entry; the other will be holding a
deleted file. By default, flock is enabled.

#### type PidFile

```go
type PidFile struct {
}
```

PidFile is a state machine that manages the lifecycle of a pidfile through
locking, publishing, destruction and unlocking.

#### func  NewPidFile

```go
func NewPidFile(opts ...ConfigOption) (*PidFile, error)
```
NewPidFile activates and optionally publishes a pidfile from a set of
configuration options.

#### func  NewPidFileWithContext

```go
func NewPidFileWithContext(ctx context.Context, opts ...ConfigOption) (*PidFile, error)
```
NewPidFileWithContext activates and optionally publishes a pidfile from a set of
configuration options, and allows a context to be provided for cancellation,
etc.

#### func (*PidFile) Activate

```go
func (pf *PidFile) Activate() error
```
Activate does as much as possible to prepare for publishing a pidfile as
possible without knowing the pid or actually publishing. This includes creating
the pidfile directory, claiming an exclusive lock (if enabled), and creating the
temporary pidfile that will be renamed over the pidfile at publishing time.

#### func (*PidFile) ActivateWithContext

```go
func (pf *PidFile) ActivateWithContext(ctx context.Context) error
```
ActivateWithContext does as much as possible to prepare for publishing a pidfile
as possible without knowing the pid or actually publishing. This includes
creating the pidfile directory, claiming an exclusive lock (if enabled), and
creating the temporary pidfile that will be renamed over the pidfile at
publishing time. ctx provides a way for the caller to abandon waiting for
completion.

#### func (*PidFile) Close

```go
func (pf *PidFile) Close() error
```
Close shuts down and deletes the PidFile, and waits for completion. Safe to call
multiple times.

#### func (*PidFile) CloseWithContext

```go
func (pf *PidFile) CloseWithContext(ctx context.Context) error
```
CloseWithContext shuts down and deletes the PidFile, and waits for completion.
ctx allows the caller to abandon waiting for completion, though once started,
the pidfile will continue shutting down.

#### func (*PidFile) GetPathName

```go
func (pf *PidFile) GetPathName() string
```

#### func (*PidFile) GetPid

```go
func (pf *PidFile) GetPid() int
```

#### func (*PidFile) Publish

```go
func (pf *PidFile) Publish() error
```
Publish finalizes publication of the pidfile with the PID determined at
configuration time. Activation is also performed if necessary.

#### func (*PidFile) PublishWithContext

```go
func (pf *PidFile) PublishWithContext(ctx context.Context) error
```
PublishWithContext finalizes publication of the pidfile with the PID determined
at configuration time. Activation is also performed if necessary. ctx provides a
way for the caller to abandon waiting for completion.

#### func (*PidFile) PublishWithPid

```go
func (pf *PidFile) PublishWithPid(pid int) error
```
PublishWithPid finalizes publication of the pidfile with a provided PID.
Activation is also performed if necessary.

#### func (*PidFile) PublishWithPidAndContext

```go
func (pf *PidFile) PublishWithPidAndContext(ctx context.Context, pid int) error
```
PublishWithPidAndContext finalizes publication of the pidfile with a provided
PID. Activation is also performed if necessary. ctx provides a way for the
caller to abandon waiting for completion.

#### func (*PidFile) Shutdown

```go
func (pf *PidFile) Shutdown(completionErr error) error
```
Shutdown shuts down a PidFile and waits for complete shutdown. If not nil,
completionErr provides a reason for the shutdown that will be returned from
subsequent API calls. Safe to call multiple times; only the first call's
completionErr is effective.

#### func (*PidFile) ShutdownWithContext

```go
func (pf *PidFile) ShutdownWithContext(ctx context.Context, completionErr error) error
```
ShutdownWithContext shuts down a PidFile and waits for complete shutdown. If not
nil, completionErr provides a reason for the shutdown that will be returned from
subsequent API calls. Safe to call multiple times; only the first call's
completionErr is effective. ctx allows the caller to abandon waiting for
completion, though once started, the pidfile will continue shutting down.

#### func (*PidFile) StartShutdown

```go
func (pf *PidFile) StartShutdown(completionErr error)
```
StartShutdown begins shutting down a PidFile if it has not already started
shutting down. Does not wait for complete shutdown. If not nil, completionErr
provides a reason for the shutdown that will be returned from subsequent API
calls. Safe to call multiple times; only the first call is effective.

#### func (*PidFile) WaitForShutdown

```go
func (pf *PidFile) WaitForShutdown() error
```
WaitForShutdown waits for a PidFile to completely shut down complete shutdown.
It does not initiate shutdown itself. The returned error is the final Close()
return value, or the context error.

#### func (*PidFile) WaitForShutdownWithContext

```go
func (pf *PidFile) WaitForShutdownWithContext(ctx context.Context) error
```
WaitForShutdownWithContext waits for a PidFile to completely shut down complete
shutdown. It does not initiate shutdown itself. ctx allows the caller to abandon
waiting for completion. The returned error is the final Close() return value, or
the context error.
<!--/tmpl-->

### Caveats

- In order so provide reliable file locking, a parallel file with a ".lock" extension is created in the same directory
  as the pidfile. This lockfile is not deleted as that would defeat its purpose.  The locking feature
  can be disabled with an option, with the consequence that two processes may claim the same pidfile, with only the last one's
  pid actually being readable.

### Contributing

- http://golang.org/doc/code.html
- http://golang.org/doc/effective_go.html
- `github.com/sammck-go/pidfile/pidfile.go` contains the importable package
- `github.com/sammck-go/pidfile/cmd/with-pidfile.go` contains the command-line wrapper tool

### Changelog

- `1.0` - Initial release
