// What engine.go asks of libunbound in C: a query whose answer comes back to Go, and a log file.
#include <stdint.h>
#include <stdio.h>
#include <unbound.h>

int offlok_resolve(struct ub_ctx *ctx, const char *name, int rrtype, int rrclass, uintptr_t key, int *id);
FILE *offlok_log_file(int fd);
