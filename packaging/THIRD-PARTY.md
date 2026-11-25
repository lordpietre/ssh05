# Componentes de terceros incluidos en el paquete ssh05

## Nmap

- Proyecto: https://nmap.org
- Versión incluida: 7.99 (binario estático embebido para Linux amd64/arm64)
- Origen del build estático: https://github.com/whoisnian/static-binaries/releases/tag/v20260901.1
- Licencia: Nmap Public Source License (NPSL); texto completo en
  `/usr/share/doc/ssh05/nmap-LICENSE` y https://nmap.org/npsl/
- Uso comercial/OEM: https://nmap.org/oem/

## Bun

- Proyecto: https://bun.sh · https://github.com/oven-sh/bun
- Versión incluida: 1.4.5 (binario `/usr/lib/ssh05/bun`)
- Licencia: MIT

## OpenTUI

- Proyecto: https://github.com/anomalyco/opentui
- Versión incluida: `@opentui/core` 0.8.2 y su paquete nativo por arquitectura
- Licencia: MIT · Copyright (c) 2025 opentui

## Dependencias JavaScript de OpenTUI

`bun-ffi-structs`, `diff`, `marked` y `string-width` se distribuyen dentro de
`usr/lib/ssh05/prototype/terminal/node_modules` con sus propias licencias. Los
textos completos están en el archivo `LICENSE` de cada paquete instalado.

El código propio de ssh05 se distribuye bajo MIT; véase
`/usr/share/doc/ssh05/copyright`.
