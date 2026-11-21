# Línea base de rendimiento

| Campo | Valor |
|---|---|
| Fecha | 22/11/2026 |
| Máquina | Intel Core i5-8265U, Linux amd64 |
| Go | 1.25 |
| Comando | `make bench` |

Los números son orientativos y sirven para detectar regresiones, no como
umbrales de aceptación. La máquina estaba compartida con otras tareas.

## Resultados

| Benchmark | Trabajo | Resultado |
|---|---|---|
| `BenchmarkBinaryFrameRoundTrip64K` | frame binario de 64 KiB, ida y vuelta | 0,19–0,26 ms · 249–343 MB/s · 5 allocs |
| `BenchmarkJSONEnvelopeRoundTrip` | envelope JSON pequeño, ida y vuelta | 20–36 µs · 48 allocs |
| `BenchmarkBackendDataPath` | 32 KiB por el buffer del backend | 0,12–0,24 ms · 139–284 MB/s · 3 allocs |
| `BenchmarkSealOpenSecret` | AES-GCM sellar y abrir un secreto | ~21 µs · 13 allocs |
| `BenchmarkRenderProxyConfig` | generar un TOML de proxy STCP | ~13 µs · 16 allocs |

## Lectura

- El canal binario sostiene cientos de MB/s en memoria; el límite práctico
  sigue siendo la red SSH y el terminal.
- El backend copia cada fragmento una vez (`3 allocs/op` por 32 KiB), dentro
  del presupuesto de buffer acotado por `--max-pending-bytes`.
- JSON se usa solo para control; el PTY nunca se serializa como JSON.
- El coste criptográfico de FRP (~21 µs por operación) es despreciable frente
  al arranque de `frpc`/`frps`.

## Cómo repetir

```sh
make bench
# o con más muestras:
CGO_ENABLED=0 go test -run '^$' -bench . -benchmem -benchtime=1s \
  ./internal/ipc ./internal/supervisor ./internal/frp
```
