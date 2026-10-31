//go:build cgo && (android || libunbound)

#include <signal.h>
#include "shim.h"
#include "_cgo_export.h"

// offlok_callback hands an answer to Go, with the key offlok_resolve was given.
static void offlok_callback(void *key, int err, struct ub_result *res) { offlokAnswer((uintptr_t)key, err, res); }

// offlok_resolve asks libunbound a question; offlokAnswer (engine.go) gets the answer, with key. SIGPIPE is blocked
// meanwhile: the first query starts libunbound's thread, which inherits this thread's signals, and writes to its
// TCP sockets; a closed one must give it EPIPE, not stop the app.
int offlok_resolve(struct ub_ctx *ctx, const char *name, int rrtype, int rrclass, uintptr_t key, int *id) {
	sigset_t pipe, old;
	sigemptyset(&pipe);
	sigaddset(&pipe, SIGPIPE);
	pthread_sigmask(SIG_BLOCK, &pipe, &old);
	int err = ub_resolve_async(ctx, name, rrtype, rrclass, (void *)key, offlok_callback, id);
	pthread_sigmask(SIG_SETMASK, &old, NULL);
	return err;
}

// offlok_log_file is where libunbound writes its log: a pipe engine.go reads line by line.
FILE *offlok_log_file(int fd) {
	FILE *f = fdopen(fd, "w");
	if (f != NULL)
		setvbuf(f, NULL, _IOLBF, 0);
	return f;
}
