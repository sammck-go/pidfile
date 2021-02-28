# gopidfile

gopidfile is a simple package for managing pidfiles associated with running processes.

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

[![Releases](https://img.shields.io/github/release/sammck/gopidfile.svg)](https://github.com/sammck/gopidfile/releases) [![Releases](https://img.shields.io/github/downloads/sammck/gopidfile/total.svg)](https://github.com/sammck/gopidfile/releases)

See [the latest release](https://github.com/sammck/gopidfile/releases/latest)


**Source**

```sh
$ go get -v github.com/sammck/gopidfile
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


### Caveats

- In order so provide reliable file locking, a parallel file with a ".lock" extension is created in the same directory
  as the pidfile. This lockfile is not deleted as that would defeat its purpose.  The locking feature
  can be disabled with an option, with the consequence that two processes may claim the same pidfile, with only the last one's
  pid actually being readable.

### Contributing

- http://golang.org/doc/code.html
- http://golang.org/doc/effective_go.html
- `github.com/sammck/gopidfile/gopidfile.go` contains the importable package
- `github.com/sammck/gopidfile/cmd/with-pidfile` contains the command-line wrapper tool

### Changelog

- `1.0` - Initial release
