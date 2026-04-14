Fix the `compressResponseWriter` in `pkg/middleware/compress.go`. There are two bugs:

1. **`writer.Close()` is called unconditionally** in `CompressWithConfig` (line 144), even when the response was small and bypassed compression (written directly to the underlying `ResponseWriter` at line 166). The gzip/deflate writer's `Close()` flushes its footer bytes to the raw response, causing garbage characters after small responses (like error JSON bodies).

2. **`headerSet` is set to `true` before the size check** (line 161), so if a first small write bypasses compression, any subsequent writes still go through the compressed writer — mixing uncompressed and compressed data in the same response.

Fix: add a `compressed bool` field to `compressResponseWriter`. Only set it to `true` when the data actually exceeds `minLength` and goes through the compressed writer. In `CompressWithConfig`, only call `writer.Close()` if `crw.compressed` is true. Move `w.headerSet = true` into both branches so the flag is always set, but keep the compression decision tracked separately.
