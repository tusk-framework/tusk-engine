// Package runtime generates the private RoadRunner application worker.
//
// Worker generation is available on Windows and Linux. Windows publishes and
// removes the owned worker through verified file handles. Linux requires
// O_TMPFILE and atomic linkat publication; if either is unavailable on the
// filesystem, WriteWorker fails without publishing. Linux Cleanup removes
// generated worker from the active path but preserves the quarantine entry under a
// random .worker-quarantine-* name and returns an error, because POSIX cannot
// unlink a directory entry conditionally on its inode identity. The caller
// must arrange exclusive access before manually removing that artifact.
//
// macOS, DragonFly, FreeBSD, NetBSD, OpenBSD, AIX, and Solaris fail closed
// before creating runtime files. Their publication and cleanup contracts are
// not claimed to have Windows-style ownership guarantees.
package runtime
