# Nmap embebido

`ssh05` incluye Nmap estático para descubrimiento de red local en Linux
`amd64` y `arm64`.

| Dato | Valor |
|---|---|
| Versión | 7.99 |
| Origen | https://github.com/whoisnian/static-binaries/releases/tag/v20260901.1 |
| Build | Alpine Linux, static-pie, stripped |
| `nmap_linux_amd64` SHA-256 | `af9ca522f69a2d58e0161992be9d94b2c25fafaacdccfa6e9b731b158f92bbd3` |
| `nmap_linux_arm64` SHA-256 | `d4aa0f8115b71738533941abb5dbff873215af65875b9ac84933273e100fd456` |

Los binarios se guardan comprimidos con gzip y se extraen a
`$XDG_CACHE_HOME/ssh05/nmap/7.99-<plataforma>/nmap` con permisos `0700`.
Antes de ejecutarlos se comprueba su SHA-256.

En otras plataformas, `ssh05d` usa el binario indicado con `--nmap`.

## Licencia

Nmap se distribuye bajo la Nmap Public Source License (NPSL). El texto
completo está en [LICENSE](LICENSE) y en https://nmap.org/npsl/.
El código de ssh05 sigue siendo MIT; Nmap es un componente separado.
Para uso comercial o OEM de Nmap, consulta https://nmap.org/oem/.
