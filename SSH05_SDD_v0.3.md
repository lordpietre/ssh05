---
title: "ssh05 — Software Design Document"
subtitle: "Arquitectura y especificación funcional · Versión 0.3"
author: "Dinámicas Electrónicas"
date: "22 de noviembre de 2026"
lang: es-ES
---

# Control del documento

| Campo | Valor |
|---|---|
| Proyecto | ssh05, cliente SSH multipestaña y supervisor local de Dinámicas Electrónicas |
| Versión | 0.3 — reunificación de identidad y avance de implementación |
| Fecha | 22/11/2026 |
| Estado | Aprobado para P0/P1/P2; la identidad pública es ssh05 (vendor Dinámicas Electrónicas) |
| Plataforma inicial | Linux; portabilidad a macOS/Windows sujeta a validación |
| Soporte | dinamicas.electronicas@gmail.com |
| Principio rector | La experiencia de conexión multipestaña es el centro del producto |

**Alcance documental.** Este SDD convierte decisiones expresadas durante la definición de ssh05 en especificaciones comprobables. No acredita que se haya implementado o probado el software ni que se haya modificado el repositorio. Las versiones de dependencias deberán bloquearse y validarse al iniciar cada fase. Las afirmaciones sobre código preexistente son hipótesis de aprovechamiento, sujetas a auditoría de la revisión concreta.

# 1. Introducción y visión del producto

ssh05 es una consola de conexiones remotas centrada en abrir, mantener, cambiar, recuperar y cerrar múltiples sesiones SSH dentro de una sola interfaz de terminal, con una experiencia de pestañas inspirada en OpenCode. Unifica el descubrimiento de hosts, la autenticación mediante KeePass/KeePassXC, la persistencia opcional de terminales remotas con tmux y la administración integral de túneles FRP.

**Propuesta de valor.** El operador elige un host; ssh05 presenta su identidad y estado, resuelve una ruta permitida —directa, ProxyJump o FRP—, obtiene autenticación a través de un proveedor autorizado y abre una pestaña independiente. Los túneles y las sesiones poseen ciclos de vida separados. La aplicación mantiene visible qué está conectado, qué se ha recuperado y qué requiere una acción del usuario.

## 1.1 Objetivos medibles

- Abrir al menos tres sesiones SSH simultáneas y alternar entre ellas sin interrumpir programas interactivos.
- Cerrar una pestaña sin afectar a las otras conexiones ni detener túneles compartidos.
- Importar perfiles desde KeePass/KDBX y `~/.ssh/config` sin copiar secretos a la base de perfiles.
- Recuperar el trabajo remoto protegido por tmux tras perder la conexión o reiniciar el cliente, cuando el servidor aún conserve la sesión.
- Crear, arrancar, observar y detener túneles FRP desde la misma interfaz, sin depender de una pestaña SSH abierta.
- Rechazar silenciosamente *ningún* cambio de identidad SSH: las claves de host nuevas o modificadas se gestionarán mediante flujos explícitos.

## 1.2 No objetivos de la primera entrega

No se implementará un protocolo SSH o FRP propio; no se promete persistencia remota sin tmux u otro soporte del servidor; no se convierte ssh05 en un escáner masivo de Internet; no se desarrollará un visor gráfico de imágenes, SIXEL ni Kitty Graphics. Las conexiones serie RS-232/RS-485/UART/USB quedan anotadas únicamente como **idea futura**, no como requisito ni *feature request* comprometida.

# 2. Alcance, prioridades y requisitos

Los requisitos se identifican mediante prefijos: `SES` (sesiones), `INV` (inventario), `AUT` (autenticación), `FRP` (túneles), `RUT` (rutas), `UI` (interfaz), `SEG` (seguridad), `OPS` (operación), `DIA` (diagnóstico) y `DAT` (transferencia). `P0` significa imprescindible para el primer núcleo útil; `P1`, requisito comprometido tras el núcleo; `P2`, capacidad secundaria prevista. La prioridad no implica un plazo.

## 2.1 Requisitos funcionales trazables

| ID | Prioridad | Requisito y resultado observable |
|---|---|---|
| SES-01 | P0 | Abrir varias pestañas SSH independientes, cada una con su sesión y terminal funcional. |
| SES-02 | P0 | Cambiar de pestaña sin suspender ni cerrar procesos de otras sesiones. |
| SES-03 | P0 | Cerrar únicamente la sesión asociada a la pestaña, previa distinción entre cerrar, desconectar y desacoplar tmux. |
| SES-04 | P0 | Propagar tamaño de terminal, teclado, señales y cambios de dimensión correctamente al PTY. |
| SES-05 | P0 | Mostrar estados `connecting`, `connected`, `degraded`, `reconnecting`, `disconnected`, `failed`, `closed`. |
| SES-06 | P1 | Reconectar mediante política configurable, con límites y cancelación. |
| SES-07 | P1 | Crear/adherir sesiones tmux remotas de manera optativa, con nombres estables y consentimiento. |
| SES-08 | P1 | Restaurar pestañas como perfiles, y reenganchar tmux solo si la sesión existe. |
| INV-01 | P0 | Mantener un inventario canónico de hosts sin credenciales embebidas. |
| INV-02 | P1 | Importar hosts desde `~/.ssh/config`, respetando `Include`, aliases y opciones aplicables. |
| INV-03 | P1 | Descubrir y sincronizar entradas KeePass/KDBX con datos SSH suficientes. |
| INV-04 | P1 | Deduplicar conservando procedencia, alias y modificaciones explícitas del usuario. |
| INV-05 | P1 | Asignar grupos, etiquetas, favoritos y colores por host o entorno. |
| AUT-01 | P0 | Admitir SSH Agent y claves gestionadas por OpenSSH. |
| AUT-02 | P1 | Usar KeePassXC SSH Agent sin extraer la clave privada del agente. |
| AUT-03 | P1 | Acceder a KDBX autorizado y desbloqueado; asociar entradas a perfiles por identificador estable. |
| AUT-04 | P1 | No persistir contraseñas, tokens ni claves privadas en los perfiles ni en los registros. |
| AUT-05 | P1 | Bloquear el acceso a secretos tras tiempo de inactividad y evitar su exposición en UI/IPC. |
| FRP-01 | P1 | Gestionar configuraciones y ciclo de vida de `frpc` y `frps` oficiales. |
| FRP-02 | P1 | Crear, editar, validar y eliminar proxies TCP, UDP, STCP y XTCP compatibles con la versión instalada. |
| FRP-03 | P1 | Generar tokens mediante CSPRNG y fallar de forma segura si falla la generación. |
| FRP-04 | P1 | Almacenar secretos protegidos; aplicar permisos restrictivos a archivos y directorios. |
| FRP-05 | P1 | Mostrar estado, destino, puerto, errores y estadísticas disponibles de cada proxy. |
| FRP-06 | P1 | Permitir rotación coordinada de autenticación, con procedimiento de recuperación. |
| FRP-07 | P1 | Comprobar firma/hash de binarios y compatibilidad antes de actualizar. |
| FRP-08 | P1 | Preservar la vida del túnel al cerrar pestañas que lo utilicen. |
| RUT-01 | P1 | Elegir rutas entre SSH directo, ProxyJump y FRP en función de la configuración autorizada. |
| RUT-02 | P1 | Probar accesibilidad sin interpretar silencio como prueba de puerto cerrado. |
| RUT-03 | P1 | Mostrar la ruta elegida; no introducir rutas alternativas inseguras ni abrir exposiciones públicas silenciosamente. |
| UI-01 | P0 | Construir la interfaz sobre TypeScript/Bun y OpenTUI, con teclado como control completo. |
| UI-02 | P1 | Admitir pestañas con colores RGB y degradación a paletas más limitadas. |
| UI-03 | P1 | Identificar el estado de conexión por símbolo y texto, independientemente del color. |
| UI-04 | P1 | Proporcionar búsqueda, paleta de comandos y atajos configurables. |
| OPS-01 | P1 | Guardar y restaurar espacios de trabajo, perfiles y disposición de pestañas. |
| OPS-02 | P1 | Separar procesos de sesión y túnel; liberar recursos sin huérfanos. |
| DIA-01 | P2 | Diagnosticar DNS, puerto SSH, banner, latencia y certificado TLS cuando proceda. |
| DIA-02 | P2 | Integrar Nmap como motor optativo, con confirmación y uso únicamente autorizado. |
| DIA-03 | P2 | Mostrar GeoIP orientativo, ASN y RDAP; indicar imprecisión y procedencia. |
| DAT-01 | P2 | Integrar transferencia de archivos SFTP, SCP y, si es compatible, trzsz/Zmodem. |
| SEG-01 | P0 | Verificar la identidad del servidor SSH; nunca aceptar automáticamente cambios de clave. |
| SEG-02 | P0 | Evitar inyección de comandos y filtración de secretos en argumentos/procesos/registros. |
| SEG-03 | P1 | Aplicar privilegios mínimos, autenticación de IPC y control de permisos en túneles. |
| SEG-04 | P1 | Registrar acciones operativas sin credenciales y con política de retención. |
| SEG-05 | P1 | Ejecutar pruebas adversariales y simulaciones de red antes de considerar estable una versión. |

## 2.2 Requisitos no funcionales

- **Fiabilidad:** una excepción o caída de una sesión no debe tirar las demás pestañas ni la supervisión FRP. Reconexión con *backoff* exponencial y dispersión; sin bucles ilimitados de alta frecuencia.
- **Seguridad:** seguridad por defecto, identidad SSH estricta, sin secretos persistidos en claro y con permisos mínimos.
- **Compatibilidad:** terminales con UTF-8 y teclado básico; degradación de True Color; Linux como objetivo de referencia.
- **Operabilidad:** diagnóstico estructurado, estados visibles, posibilidad de exportar un informe saneado y operaciones cancelables.
- **Rendimiento:** interfaz fluida con tres o más sesiones; límites configurables de buffers y colas de eventos. Los umbrales exactos de memoria y latencia se fijarán tras establecer una línea base.
- **Accesibilidad:** estado semántico y no solo cromático, navegación completa por teclado, foco visible.

# 3. Arquitectura general

Se adopta **arquitectura hexagonal con adaptadores de transporte**. La interfaz nunca manipula directamente secretos ni procesos FRP; solicita operaciones tipadas al servicio de aplicación. La separación permite sustituir el backend Go por otro en el futuro sin alterar el contrato visual.

```text
┌──────────────────────────────────────────────────────────────┐
│ ssh05: OpenTUI + TypeScript + Bun                             │
│ pestañas · hosts · rutas · perfiles · FRP · espacios trabajo  │
└─────────────────────────┬────────────────────────────────────┘
                          │ IPC local tipado y versionado
┌─────────────────────────▼────────────────────────────────────┐
│ Coordinador de aplicación / motor de conexiones (Go inicial)  │
│ perfiles · sesiones · routing · credenciales · supervisión    │
├──────────────┬───────────────┬──────────────┬─────────────────┤
│ SSH/PTY      │ KeePass/Agent │ FRP manager  │ diagnóstico     │
│ OpenSSH      │ KDBX/SSH_AUTH │ frpc / frps  │ DNS/ASN/RDAP    │
└──────┬───────┴──────┬────────┴──────┬───────┴─────────────────┘
       │              │               │
   servidores      proveedor      proxies y redes
   + tmux remoto   autorizado     privadas
```

**Decisión inicial:** interfaz TypeScript/Bun/OpenTUI y núcleo Go. No se exige conservar las bibliotecas existentes de `trzsz-ssh`; se valorarán frente a OpenSSH externo y otras implementaciones por compatibilidad, coste y pruebas. Rust queda como alternativa futura, no como decisión de reescritura. FRP se consume a través de ejecutables oficiales en primera instancia, sin copiar ni reimplementar su protocolo.

## 3.1 Límites de proceso

- Proceso UI: render, foco, comandos y caché de vistas no secreta.
- Servicio de aplicación: perfiles, políticas, multiplexación de eventos, autorización, ciclo de vida.
- Cada SSH: transporte/PTY/flujo asociado a una identidad de sesión; preferible proceso aislable o adaptador con aislamiento equivalente.
- Cada FRP: proceso o servicio supervisado independiente de los procesos SSH.
- KeePassXC: proveedor externo; se minimizan los secretos entregados al servicio de aplicación.

# 4. Gestor de sesiones SSH y pestañas

## 4.1 Modelo y estados

`HostProfile` describe un destino lógico; `ConnectionRoute`, cómo llegar; `Session`, una conexión concreta; `Tab`, la representación visual. Una sesión puede tener un `TmuxBinding` opcional. Varias sesiones pueden referirse al mismo perfil y varias pestañas pueden operar sobre el mismo host sin compartir PTY.

La máquina de estados básica es: `created → connecting → connected → [degraded] → reconnecting → connected` o `failed/disconnected → closed`. `closed` es terminal. Se distinguen **desconectar** (terminar transporte), **cerrar pestaña** (cerrar la vista y, por defecto, terminar transporte asociado) y **desacoplar tmux** (conservar ejecución remota y cerrar transporte local). Antes de cerrar una sesión con trabajo activo se permite configurar una confirmación.

## 4.2 Transporte y emulación de terminal

El adaptador SSH deberá soportar PTY, cambio de tamaño, señales, bytes arbitrarios, estado de salida y `stdin/stdout/stderr`. El diseño de OpenTUI debe probar cuidadosamente la convivencia entre la TUI anfitriona y una terminal anidada que ejecuta `vim`, `top`, `nano` o `tmux`; no se asume que un control de texto de OpenTUI sea un emulador VT completo. Se decidirá tras prototipo entre emulación VT con buffer propio, un componente disponible de OpenTUI o una capa terminal dedicada.

## 4.3 Política de reconexión

La reconexión no puede prometer recuperar un shell convencional ya terminado. Ante un corte, se marca la pestaña y se reintenta establecer el transporte de forma acotada; si existe vínculo tmux, se intenta reenganchar con validación. Se distinguen reconexión de red, nueva sesión SSH y recuperación del estado remoto.

# 5. Persistencia, tmux y redes inestables

La persistencia real del trabajo remoto depende de procesos que permanezcan activos en el servidor, normalmente `tmux`. ssh05 registra el nombre/identificador de la sesión tmux y la identidad del host, nunca asume que el shell siga existiendo. El modo tmux es opcional y requiere disponibilidad y autorización en el servidor.

**Flujo de recuperación:** comprobar host y clave pública → resolver ruta → autenticar → comprobar tmux remoto → reenganchar si coincide → avisar si no existe. Tras reiniciar ssh05, se restauran las pestañas **como vistas y perfiles**, no como conexiones de red mágicamente vivas.

El soporte UDP del proyecto preexistente, QUIC/KCP o Mosh son opciones de evaluación separadas; no se equiparan con tmux ni se introducen sin validar compatibilidad, requisitos del servidor y modelo de amenaza.

# 6. Inventario inteligente de servidores

## 6.1 Fuentes y precedencia

- Manuales: el usuario conserva la autoridad sobre modificaciones locales.
- `~/.ssh/config`: importación y lectura controlada, con cuidado con `Include`, `Match` y expansiones.
- KeePass/KDBX: importación de entradas con campos explícitos o reconocibles de SSH.

La coincidencia no depende solo de `hostname`: se comparan usuario, puerto, ruta, identificador fuente y alias. Los perfiles fusionados conservan **provenance** de cada valor, historial y posibilidad de separar entradas. Los cambios de una fuente externa no deben sobrescribir silenciosamente modificaciones locales.

## 6.2 Ejemplo de perfil

```json
{
  "id": "host-92",
  "alias": "laboratorio-arm64",
  "target": {"hostname": "lab.example.net", "port": 2222, "username": "admin"},
  "sourceRefs": [{"kind": "keepass", "entryId": "uuid-externo"}],
  "authRef": {"provider": "keepass", "entryId": "uuid-externo"},
  "routePolicyId": "rp-lab",
  "labels": ["laboratorio", "arm64"],
  "color": "#5878CC"
}
```

Ningún perfil contiene contraseñas o claves privadas. Un host importado figura como **descubierto/no verificado** hasta comprobar conectividad e identidad.

# 7. Autenticación y compatibilidad KeePass/KeePassXC

ssh05 reconoce KDBX como fuente de perfiles y credenciales; KeePassXC es un proveedor posible, no un servicio de base de datos. Se prioriza la integración con SSH Agent cuando se usa autenticación por clave; para contraseñas se contempla una interfaz autorizada y desbloqueada que reduzca el tiempo de exposición en memoria. La lectura directa de KDBX necesita un parser fiable, política de desbloqueo y formato/versiones compatibles; se implementará solamente tras evaluación de biblioteca y amenazas.

**Detección automática:** buscar entradas con `URL`, `Host`, `Hostname`, `UserName`, `Port`, campos personalizados o convenciones documentadas; analizar esquemas `ssh://`; validar puerto 1–65535, IDNA/IPv6 y evitar interpretar cualquier dirección como un servidor SSH. Si el puerto está ausente, utilizar 22 como valor sugerido y señalar la inferencia. Importar perfiles sin iniciar conexiones y sin prometer validez de datos.

**Sincronización:** asociar entrada por UUID de la base y huella no secreta; reconciliar cambios, eliminaciones y conflictos. Si una entrada desaparece, ofrecer desvincular o conservar como perfil manual, sin borrar automáticamente trabajo del usuario.

**Seguridad:** no registrar valores KDBX protegidos; no introducir secretos en argumentos de línea de comandos ni variables ambientales cuando exista vía más segura; restringir duración y superficie del secreto, limpiar buffers cuando sea viable y asumir los límites de garantía de borrado de memoria en runtimes gestionados.

# 8. Integración vertical de FRP

FRP es un módulo operativo central, no un submenú de sesiones SSH. Se basa en `frpc` y `frps` oficiales con configuración declarativa controlada, detección de capacidades por versión y supervisión de procesos. Se contempla la línea FRP 0.72.0 como candidata fechada de referencia (04/11/2026), sin fijarla sin verificación de publicación, hashes y compatibilidad al implementar.

## 8.1 Entidades

`FrpEndpoint` (instalación local/remota), `FrpProxy` (servicio y reglas), `FrpCredentialRef` (referencia secreta), `FrpInstance` (proceso/servicio vivo) y `FrpBinding` (asociación con una ruta SSH). Los túneles tienen estados `stopped`, `starting`, `online`, `degraded`, `restarting`, `error`, `stopping`.

## 8.2 Controles de seguridad obligatorios

- Generación criptográfica de tokens con CSPRNG y **error bloqueante** si falla; nunca token por defecto fijo.
- Archivos de configuración con secretos en modo `0600` y directorios privados `0700` en sistemas POSIX, atendiendo a ACL y propietarios.
- Autenticación y TLS configurables de manera segura; preferir acceso privado STCP si el escenario no requiere exposición pública.
- Acción explícita y resumen de impacto antes de publicar un puerto de Internet; restricciones de interfaz/ACL cuando sea viable.
- Actualizaciones con artefactos oficiales, comprobación de integridad y reversión documentada, no ejecuciones ciegas de scripts remotos.
- Supervisión independiente: cerrar una pestaña no mata `frpc` ni `frps` si atienden otras rutas; detenerlos requiere acción específica.

## 8.3 Flujos

**Crear proxy:** elegir endpoint → seleccionar tipo (TCP/UDP/STCP/XTCP según versión y soporte) → introducir destino y controles → validar colisiones y exposición → crear/generar secreto → vista previa saneada → aplicar → comprobar estado.

**Rotar token:** preparar nuevo secreto en proveedor seguro → coordinar cliente y servidor → reiniciar/recargar según capacidad comprobada → validar conectividad → invalidar el anterior. Evitar presentar una rotación como atómica si no lo es.

**Gestionar instalaciones existentes:** descubrir instancias que el usuario autorice, importarlas inicialmente en modo lectura y pedir confirmación antes de asumir su gestión o reescribir su configuración.

# 9. Resolución inteligente de conexiones

El resolver recibe `HostProfile`, `RoutePolicy`, contexto local y restricciones de seguridad. Crea un plan reproducible con candidatos priorizados, no ejecuta automáticamente comandos de configuración remota. Evalúa ruta directa, saltos declarados y túneles FRP registrados, sin efectuar escaneos amplios.

**Reglas:** solo usar rutas explícitamente autorizadas; no rebajar autenticación ni aceptar claves host nuevas de forma automática; solicitar acción antes de arrancar servicios o exponer puertos; emitir diagnóstico legible de por qué se eligió o rechazó cada candidato. Si varias rutas son viables, aplicar prioridad configurada y recordar elección por perfil cuando esté permitido.

```text
Seleccionar host → validar perfil → consultar proveedor de secreto
                → resolver rutas autorizadas → comprobar túnel necesario
                → verificar identidad SSH → abrir PTY → crear pestaña
```

# 10. Descubrimiento y diagnóstico de hosts

Capacidad complementaria, no condición para el núcleo de pestañas: consultas DNS, resolución inversa, reachability TCP, banner SSH, metadatos TLS y datos ASN/RDAP. GeoIP es aproximado y nunca se usa para inferir identidad física ni confianza. Nmap puede actuar como motor optativo para entornos administrados por el usuario, con límites de velocidad, alcance explícito y cancelación. La ausencia de respuesta no se comunica automáticamente como «cerrado».

Las direcciones resueltas por un dominio son dependientes del momento y pueden apuntar a CDNs o balanceadores. Los resultados guardan fecha, IP evaluada, origen y grado de certeza.

# 11. Interfaz OpenTUI

La interfaz prioriza la terminal interactiva y mantiene un panel de comandos discreto. Las pestañas muestran nombre del host, estado textual/iconográfico y color de entorno. La selección no debe confundirse con el estado de conexión. El soporte True Color es una mejora progresiva, con degradación y sin dependencia exclusiva del color.

**Áreas funcionales:** selector de hosts; barra de pestañas con alta/cierre/reordenación; área de terminal VT; panel de rutas; gestor FRP; aviso de seguridad; paleta de comandos; espacios de trabajo. Acciones principales disponibles por teclado. Atajos sugeridos, ajustables: `Ctrl+Shift+T` crear pestaña, `Ctrl+Tab` siguiente, `Ctrl+Shift+W` cerrar con confirmación pertinente. Se hará una auditoría de colisiones con controles comunes de terminal y remotos.

La vista de sesiones debe mostrar errores seguros y mensajes de reconexión sin inyectarlos como comandos en el terminal remoto.

# 12. Espacios de trabajo

Un `Workspace` conserva lista ordenada de pestañas previstas, host, preferencias visuales, política de ruta, identificador tmux opcional y referencias a túneles. No serializa bytes de la sesión SSH, credenciales, buffers de terminal con datos sensibles ni secretos. Al restaurar, ssh05 resuelve cada ruta de nuevo y pide desbloqueo de secretos cuando corresponda.

Se ofrecerán estrategias de apertura: manual, reconectar perfiles previamente activos o intentar recuperar tmux. El usuario podrá configurar confirmación para conectar automáticamente y para iniciar túneles, y siempre se respetará la verificación de host.

# 13. Transferencias y operaciones de archivos

El motor de transferencia estará desacoplado de la UI, con progresos y cancelación. Se evaluarán SFTP/SCP y la compatibilidad trzsz/Zmodem existentes. La transferencia tendrá su identidad, archivos de origen/destino, comprobación de errores, progreso y manejo de sobrescritura segura. Nunca se asumirá que el shell remoto ofrece una ruta confiable sin validación.

Queda fuera del alcance inicial un explorador gráfico de imágenes, Sixel y previsualizaciones multimedia.

# 14. Seguridad y modelo de amenazas

## 14.1 Activos y adversarios

Activos: secretos de KeePass, identidad de servidores, sesiones de administración, configuraciones FRP, privilegios del usuario, registros y ficheros transferidos. Amenazas: MITM SSH, robo de tokens, inyección de comandos, actualización comprometida, ataque local a IPC, exposición accidental de puertos, rutas equivocadas, filtración en logs y persistencia de procesos no supervisados.

| Amenaza | Mitigación de diseño | Verificación |
|---|---|---|
| Clave SSH host alterada | Verificación estricta con aviso y decisión explícita | Prueba con clave modificada |
| Token FRP predecible | CSPRNG sin fallback fijo | Inyección de fallo RNG |
| Archivo FRP legible por terceros | Modo 0600 y directorios 0700/ACL | Inspección de permisos |
| Exposición accidental | Vista previa y confirmación de puerto público | Prueba de aceptación UI |
| Inyección en host/puerto | Parámetros tipados, sin concatenación a shell | Fuzzing de entradas |
| Robo o eco de secreto en logs | Redacción centralizada y pruebas de snapshots | Análisis de trazas |
| IPC no autorizado | Socket local restringido, handshake y autorización por usuario | Cliente local no autorizado |
| Procesos huérfanos | Supervisor con propiedad y reconciliación | Cierre abrupto y reinicio |
| Confusión de rutas | Rutas autorizadas y exposición de la ruta efectiva | Simulación de failover |
| Dependencia maliciosa | Versiones fijadas, comprobación de procedencia, SBOM | Pipeline de release |

## 14.2 Política de mínimos

La configuración deberá evitar guardar credenciales en claro; el IPC no admitirá operaciones arbitrarias de ejecución como API genérica; las acciones remotas privilegiadas serán auditables y explícitas; las actualizaciones no se activarán sin control del usuario. Se documentarán los riesgos residuales de dependencias y componentes externos.

# 15. Pruebas, validación y aceptación

## 15.1 Matriz esencial

| Caso | Resultado exigido |
|---|---|
| Tres SSH simultáneas | Tres shells funcionales, alternancia sin interrupción |
| Cierre de pestaña B | A y C siguen activas; túnel compartido sigue según política |
| `vim`/`htop` y resize | Render y control del PTY correctos tras redimensionar |
| Pérdida de red | Estado visible; reintentos acotados; sin bloqueo de UI |
| tmux remoto | Reattachment correcto si sesión existe; mensaje si no |
| KeePass import | Host, puerto, usuario importados; cero secretos en perfil |
| KeePass conflict | Cambios detectados sin sobrescritura silenciosa |
| Host key cambia | Conexión bloqueada hasta decisión explícita |
| FRP token RNG falla | No se crea proxy ni token predeterminado |
| FRP con secreto | Permisos restrictivos y logs sin valor secreto |
| Ruta FRP ausente | Mostrar requisito y pedir acción, sin exposición silenciosa |
| Reinicio de ssh05 | Workspace restaurado como vistas; reconexión conforme a política |
| Caída proceso SSH | Otras sesiones y supervisor FRP sobreviven |
| Actualización FRP inválida | Bloqueo, error útil y conservación de versión anterior |

## 15.2 Pruebas de fallos

Emular latencia, pérdida de paquetes, DNS intermitente, cortes TCP, sustitución de clave host, expiración de autenticación, bloqueo KeePass, fallo de IPC y caídas de `frpc`/`frps`. Ensayar Linux con tmux presente/ausente, sin agente, con agente y con privilegios restringidos. Añadir integración CI de pruebas unitarias, sanitización de logs y análisis estático; antes de release, pruebas reales con varias redes y destinos de laboratorio.

# 16. Estrategia de implementación

## 16.1 Fases y condiciones de salida

**Fase 0 — Auditoría y prototipo de terminal.** Inventariar código original y licencias, elegir implementación SSH/PTY, demostrar que OpenTUI puede alojar terminales VT interactivas sin interferir con ellas. Salida: prototipo `vim` + `htop` + resize.

**Fase 1 — Núcleo multipestaña P0.** UI, IPC, supervisor, perfiles mínimos, tres sesiones simultáneas, cierre independiente, claves SSH verificadas. Salida: suite P0 aprobada.

**Fase 2 — Inventario y KeePass.** `ssh_config`, KDBX autorizado, deduplicación, provenance, agente SSH y secretos temporales. Salida: importar/sincronizar de forma segura y abrir sesión.

**Fase 3 — FRP vertical.** Adaptador oficial `frpc`/`frps`, tokens seguros, tipos de proxy, supervisión, asociación a perfiles, actualización verificada y resolución de rutas. Salida: conectar a un host privado por ruta FRP autorizada sin exponer secretos.

**Fase 4 — Persistencia y espacios de trabajo.** tmux opcional, recuperación, política de reconexión y restauración de disposición. Salida: cerrar y abrir ssh05 sin perder el trabajo remoto protegido por tmux.

**Fase 5 — Diagnóstico, transferencias y endurecimiento.** Nmap optativo, GeoIP/ASN/RDAP, archivos y pruebas de seguridad/red. Salida: release candidate con criterios de aceptación y documentación.

## 16.2 Riesgos y decisiones abiertas

| ID | Decisión pendiente | Criterio de resolución |
|---|---|---|
| ADR-01 | OpenSSH externo frente a motor SSH Go existente | Compatibilidad real, PTY, señales, agent y seguridad |
| ADR-02 | Emulador VT embebido en OpenTUI | Prueba con herramientas interactivas y mouse/resize |
| ADR-03 | IPC (JSON-RPC o protobuf) | Facilidad de depuración, backpressure y control de acceso |
| ADR-04 | Gestor de secretos KDBX | Bibliotecas compatibles, validación y superficie de exposición |
| ADR-05 | Ownership de procesos FRP al cerrar ssh05 | Política de servicio persistente vs proceso hijo supervisado |
| ADR-06 | Esquema de sincronización de hosts | Conflictos, provenance, cambios y borrados |
| ADR-07 | Transportes UDP opcionales | Seguridad, interoperabilidad y coste de despliegue remoto |
| ADR-08 | Política de autoarranque de túneles | Consentimiento, alcance y restricciones de acceso |

# Anexo A. Modelo de dominio

Entidades principales y relaciones:

- `HostProfile` 1:N `Session`; 1:N `SourceReference`; 0:N `Label`.
- `Session` 1:1 `Tab` en vista principal; 0:1 `TmuxBinding`; 1:1 `ConnectionRoute` elegida.
- `RoutePolicy` 1:N `ConnectionRoute` candidatas; una ruta FRP puede referir a `FrpProxy` compartido.
- `FrpEndpoint` 1:N `FrpProxy`; `FrpProxy` 0:N `FrpBinding`.
- `Workspace` 1:N `WorkspaceTabRecord`; cada registro refiere a perfil y opciones no secretas.
- `CredentialReference` identifica proveedor y entrada, nunca el valor del secreto.

**Invariantes:** cerrar `Tab` no implica detener `FrpProxy`; `CredentialReference` nunca contiene la contraseña; la aceptación de una host key es acción específica y auditada; borrar un perfil no borra necesariamente un túnel compartido.

# Anexo B. Contrato IPC inicial

El IPC transporta comandos y eventos tipados con `requestId`, `protocolVersion`, `sessionId`, `timestamp` y `errorCode` cuando corresponda. Debe respetar orden por sesión, colas con límites y cancelación.

| Comando | Resultado/eventos |
|---|---|
| `hosts.list`, `hosts.import`, `hosts.resolve` | Perfiles, conflictos y provenance |
| `sessions.open`, `sessions.write`, `sessions.resize`, `sessions.close` | Estados y bytes PTY por sesión |
| `sessions.detachTmux`, `sessions.attachTmux` | Vínculo remoto o error explícito |
| `routes.plan`, `routes.connect` | Candidatos y ruta autorizada usada |
| `frp.instances.list`, `frp.proxies.apply`, `frp.proxies.stop` | Estado y errores saneados |
| `vault.status`, `vault.unlockRequest` | Estado/autorización; nunca secreto bruto en UI |
| `workspaces.save`, `workspaces.restore` | Disposición y planes de recuperación |

**Ejemplo indicativo (no API final):**

```json
{"protocolVersion":1,"requestId":"req-41","method":"sessions.open",
 "params":{"hostProfileId":"host-92","routePolicyId":"rp-lab"}}
```

Envío de bytes PTY mediante canal separado o encuadre binario; no serializar grandes cantidades de output como eventos JSON ilimitados. El handshake autentica extremos locales y negocia versiones.

# Anexo C. Flujos de secuencia críticos

**Conectar:** UI → resolver perfil → plan de ruta → solicitar secreto/agent → preparar túnel autorizado si procede → verificar host key SSH → abrir SSH/PTy → asignar tab → recibir output.

**Desconexión:** detector del transporte → marcar pestaña → plan de reconexión → SSH nuevo → `tmux attach` solo cuando corresponda → confirmar continuidad de sesión.

**Rotación FRP:** confirmar alcance → generar secreto seguro → preparar endpoints → aplicar cambios controlados → prueba de conexión → retirar credencial antigua o informar de recuperación.

# Anexo D. Ideas fuera del alcance inicial

- Conexiones serie (RS-232, RS-485, UART y USB-serie) como posibles transportes futuros de pestañas, **sin compromiso de implementación**.
- Explorador visual de imágenes, SIXEL y Kitty Graphics: expresamente descartados por ahora.
- Otras posibilidades sin aprobar: ejecución de playbooks, paneles Docker, monitorización avanzada sin agente.

# Anexo E. Dependencias, licencias y atribución

ssh05 parte de las ideas y del repositorio de referencia `https://github.com/lordpietre/ssh05`. Si se reutilizan fuentes de terceros, conservar los avisos originales y verificar la licencia de cada componente y dependencia; el nombre nuevo no elimina deberes de atribución. El código original de `trzsz-ssh` se distribuye bajo MIT; FRP bajo Apache 2.0; las condiciones de KeePassXC, bibliotecas KDBX y datos GeoIP requieren revisión específica para distribución. No se ha efectuado cambio ni publicación de repositorio en este documento.

# Anexo F. Glosario y fuentes de referencia

**Host:** destino lógico registrado. **Conexión:** transporte activo a un destino. **Sesión:** ejecución remota interactiva asociada a PTY. **Pestaña:** vista de UI. **Túnel:** encaminamiento de un servicio, potencialmente compartido. **Ruta:** modo autorizado de alcanzar un host. **tmux:** persistencia de procesos remotos. **FRP:** proxy inverso gestionado. **KDBX:** formato cifrado de KeePass. **Provenance:** procedencia de un valor importado.

Referencias primarias (consultar revisión y fecha al implementar):

1. Proyecto de referencia ssh05: https://github.com/lordpietre/ssh05
2. Proyecto upstream: https://github.com/trzsz/trzsz-ssh
3. OpenTUI: https://github.com/anomalyco/opentui
4. FRP, versiones y documentación: https://github.com/fatedier/frp/releases ; https://gofrp.org/
5. KeePassXC: https://keepassxc.org/docs/
6. OpenSSH: https://www.openssh.com/manual.html
7. tmux: https://github.com/tmux/tmux/wiki
8. RFC 4251 (arquitectura SSH): https://www.rfc-editor.org/rfc/rfc4251

**Criterio de cierre del SDD v0.1:** revisión del prototipo PTY/OpenTUI, auditoría de código y dependencias reales, selección registrada de ADR-01 a ADR-04 y ratificación de las prioridades por el responsable del proyecto.


---

# 17. Actualización técnica 0.2: terminal embebido y PTY

## 17.1 Origen y alcance de la revisión

Este capítulo incorpora la investigación «ssh05 — Investigación: TUI, pestañas y control de sesiones SSH» (20/11/2026, Dinámicas Electrónicas). El informe afirma haber probado `EmbeddedTerminalRenderable` de `@opentui/core` 0.8.2 con salida ANSI, cursor y buffer, y `Bun.spawn({ terminal })` en Bun 1.4.4 con resize y recepción de `SIGWINCH`. **Son resultados comunicados por la investigación aportada**, no una reproducción independiente de las pruebas en esta revisión del SDD. Antes de cerrar una ADR, repetirlos con las versiones fijadas en CI y con una sesión SSH real.

### Evidencia, decisión y límites

| Tema | Evidencia del informe | Decisión de diseño | Validación restante |
|---|---|---|---|
| Emulador VT | `EmbeddedTerminalRenderable` interpreta ANSI y mantiene cursor/buffer | Adoptar un adaptador de renderable nativo, sin emulador propio | `vim`, `htop`, `tmux`, Unicode, alt-screen, selección, pegado y resize dentro de la TUI completa |
| PTY | `Bun.spawn({terminal})` y `resize()` probados | Backend PTY Bun para prototipo A | Cierre de árbol de procesos, backpressure, desconexión y señales en producción |
| JSX/Solid | El componente requiere registro `extend` o montaje Core | Crear `TerminalViewAdapter` | Montaje/desmontaje, props iniciales, limpieza y focos |
| Pestañas | Implementación propia de OpenCode, no widget de OpenTUI | Diseñar `TabStore` independiente del renderer | Overflow, drag, restauración, teclas y lectores de pantalla |
| Plataformas | Artefactos Ghostty citados para x86_64/aarch64 | Linux x86_64/aarch64 objetivo inicial | Probar glibc/musl, empaquetado y matriz real de binarios |
| `@opentui/ssh` | Sirve TUIs sobre SSH | No utilizar como transporte SSH saliente | Ninguna para el núcleo |

**ADR-02** pasa de «riesgo sin solución identificada» a **«solución candidata identificada, validación de integración pendiente»**. No debe constar como cerrada definitivamente hasta superar las pruebas de extremo a extremo.

## 17.2 Separación entre pestaña, vista, PTY y sesión

Una **pestaña** es una representación visual; una **sesión** es una conexión/ejecución gestionada; un **PTY** es el terminal del proceso; un **túnel FRP** es un recurso de conectividad compartible; una **sesión tmux** existe en el servidor remoto. Ninguna identidad debe inferirse a partir del índice visual de pestaña.

```text
OpenTUI / Solid
 ├─ TabBar / TabStore / WorkspaceStore
 ├─ HostPicker / CommandPalette / Inspector
 └─ TerminalViewAdapter → EmbeddedTerminalRenderable
             │ bytes / foco / resize
       SessionBackend (contrato estable)
             │
     ┌───────┴─────────────────────┐
     │ A: Bun PTY local            │ B: supervisor persistente
     │ prototipo, no persistente   │ Go + PTY + SSH + IPC
     └──────────────┬──────────────┘
                    │ SSH / tssh / OpenSSH
              host remoto / tmux
                    │
       rutas autorizadas: directo / ProxyJump / FRP
```

La transición A→B no cambia la semántica de `TabStore`. El backend debe notificar su capacidad real: `supportsDetach`, `supportsReattach`, `supportsRemoteTmux`, `supportsPersistentProcess` y `supportsBinaryStreaming`.

## 17.3 Contrato de sesiones revisado

```ts
type SessionId = string;
type TabId = string;
type WorkspaceId = string;
type SessionPhase =
  | 'created' | 'resolving' | 'connecting' | 'authenticating'
  | 'connected' | 'degraded' | 'reconnecting' | 'disconnected'
  | 'detached' | 'failed' | 'closing' | 'closed';

type CloseIntent = 'close-view' | 'disconnect-ssh' | 'detach-remote-tmux';

interface SessionBackend {
  open(profileId: string, routeId?: string): Promise<SessionId>;
  write(id: SessionId, bytes: Uint8Array): Promise<void>;
  resize(id: SessionId, cols: number, rows: number): Promise<void>;
  requestClose(id: SessionId, intent: CloseIntent): Promise<void>;
  attach(id: SessionId, cursor?: bigint): Promise<void>;
  onData(id: SessionId, cb: (bytes: Uint8Array) => void): () => void;
  onStatus(id: SessionId, cb: (phase: SessionPhase) => void): () => void;
  capabilities(): Promise<Record<string, boolean>>;
}
```

`close-view` **no implica** destruir el proceso si el backend soporta desacoplarlo; si no lo soporta, la UI debe advertir del efecto. `detach-remote-tmux` desacopla el cliente de tmux sin afirmar que SSH o PTY locales sigan vivos. Las transiciones se registran como eventos con ID monotónico y fecha; jamás se registra el flujo de bytes del terminal por defecto.

## 17.4 Flujo de teclado, ratón y resize

- **Foco terminal:** las teclas no reservadas van al renderable. `Escape` se entrega al programa remoto; nunca cierra una conexión por sí solo.
- **Atajos globales:** captura previa a la entrega al hijo; `stopPropagation` solo si se ejecuta una acción de ssh05. Teclas de control configurables y modo de escape/prefijo para resolver conflictos con aplicaciones remotas.
- **Pegado:** respetar bracketed paste cuando corresponda, limitar tamaño, advertir ante pegados multilinea potencialmente destructivos sin registrar su contenido.
- **Resize:** calcular tamaño del panel de terminal, aplicar dimensiones al renderable y enviar `PTY.resize(cols,rows)` con coalescencia para evitar tormentas de eventos.
- **Mouse:** los eventos pertenecen a la aplicación remota cuando esta habilita mouse reporting; en otro caso permiten selección/scroll local. Drag de pestañas solo en la barra, nunca en el viewport SSH.
- **Pestañas ocultas:** seguir drenando el PTY y aplicando límites de buffer; reducir renderizados, no detener lectura, para evitar bloqueo del proceso remoto.

## 17.5 IPC y supervisor

El supervisor B deberá existir fuera del ciclo de vida del renderer. Se propone IPC local autenticado con mensajes tipados y versionados, canal de control separado de flujo binario PTY y control de presión mediante buffers acotados. Los permisos del socket/pipe deben limitarse al usuario y deben verificarse las credenciales del par cuando el sistema lo permita. La UI nunca envía comandos de shell interpolados; solicita operaciones tipadas. El supervisor aplica autorización, auditoría saneada, límites por sesión y gestión de procesos hijos. Una caída del renderer no debe matar sesiones persistentes ni túneles independientes.

---

# 18. Sistema de diseño UX/UI de ssh05

## 18.1 Principios de producto

1. **Terminal primero:** la terminal SSH ocupa la mayor parte de la pantalla. El gestor no compite con el contenido remoto.
2. **Un host, una acción:** desde el inventario, seleccionar y conectar; la ruta y la autenticación se resuelven con transparencia.
3. **Estado inequívoco:** texto, símbolo y color comunican estados; el color por sí solo nunca decide el significado.
4. **Operaciones reversibles:** cerrar una vista, desconectar SSH, desacoplar tmux y detener FRP son acciones diferentes.
5. **Información progresiva:** mostrar lo esencial de inmediato; detalles técnicos en un inspector lateral o diálogo.
6. **Seguridad visible, sin ruido:** host key cambiada, exposición pública FRP y errores de autenticación reciben tratamiento específico; no se disfrazan como notificaciones genéricas.
7. **Teclado completo:** todas las acciones accesibles sin ratón; ratón como mejora opcional.

## 18.2 Estructura de navegación

Áreas de primer nivel: **Sesiones**, **Hosts**, **FRP**, **Espacios de trabajo** y **Diagnóstico**. La paleta global ofrece acciones transversales: «Conectar a host», «Nueva pestaña», «Abrir túnel», «Cambiar workspace», «Reenganchar tmux», «Buscar host» y «Preferencias». El área activa y la ruta actual deben ser identificables sin depender del color.

### Boceto A: sesión SSH principal (terminal 120×36)

```text
┌ ssh05 · Laboratorio ───────────────────────────────────────────────────────────┐
│ [1 ● lab-arm64] [2 ● vps-web] [3 ↻ backup] [4 ! prod-db]  ‹2   1›  [+]     │
├───────────────────────────────────────────────────────────────────────────────┤
│ Hosts ▸                 │ root@lab-arm64:~#                                   │
│ ★ lab-arm64     ●       │ $ htop                                              │
│   vps-web       ●       │                                                     │
│   backup        ↻       │                 TERMINAL REMOTO                    │
│   prod-db       !       │                                                     │
│                         │                                                     │
│ [/] Buscar host         │                                                     │
│                         │                                                     │
├─────────────────────────┴─────────────────────────────────────────────────────┤
│ SSH ● Conectado  ·  lab-arm64:22  ·  vía ProxyJump bastion  ·  tmux: dev      │
│ Ctrl+P Paleta   Ctrl+Tab Cambiar   Ctrl+Shift+W Cerrar   F1 Ayuda           │
└───────────────────────────────────────────────────────────────────────────────┘
```

El panel Hosts se puede ocultar. En modo enfoque, la barra de pestañas y la terminal permanecen, mientras la información auxiliar se condensa en una línea. La barra de estado informa de **ruta**, **identidad**, **modo tmux** y **conectividad**; nunca imprime contraseñas ni tokens.

### Boceto B: selector de host / conexión

```text
┌ Conectar a un host ────────────────────────────────────────────────┐
│ Buscar: [lab____________]  Fuente: Todas  Grupo: Laboratorio       │
│                                                                    │
│ ★ lab-arm64    admin@10.0.0.18:22     KeePass + ssh_config        │
│   lab-gateway  root@gateway.example  ssh_config                  │
│   lab-nas      admin@nas.local:2222  KeePass (sin verificar)     │
│                                                                    │
│ Ruta propuesta: ProxyJump → gateway.example → lab-arm64           │
│ Autenticación: SSH Agent (KeePassXC)                               │
│ Estado: identidad SSH previamente verificada                      │
│                                                                    │
│ [Enter] Conectar   [Tab] Detalles   [Esc] Cancelar                 │
└────────────────────────────────────────────────────────────────────┘
```

La lista distingue **origen del perfil** de **identidad del servidor verificada**. Si se usa un puerto inferido (22), debe indicarse en los detalles. La conexión solo se abre por acción del usuario.

### Boceto C: centro FRP

```text
┌ FRP · Conexiones y proxies ────────────────────────────────────────┐
│ Servidores  2   Clientes  4   Proxies activos  6   Errores  1      │
├────────────────────────────────────────────────────────────────────┤
│ ● lab-ssh-stcp    STCP  lab-arm64:22   acceso privado   [Detalles] │
│ ● web-demo        TCP   127.0.0.1:8080 expuesto        [Detalles] │
│ ! prod-backup     STCP  error de autenticación         [Revisar]  │
├────────────────────────────────────────────────────────────────────┤
│ [N] Nuevo proxy  [E] Editar  [R] Reiniciar  [T] Tokens  [V] Logs  │
└────────────────────────────────────────────────────────────────────┘
```

**Asistente de nuevo proxy:** (1) servidor frps autorizado, (2) cliente frpc, (3) tipo y servicio destino, (4) alcance de exposición y autenticación, (5) validación y previsualización, (6) aplicar, probar y confirmar. Para TCP público, presentar advertencia de exposición, dirección/puerto resultantes y confirmación explícita. Para STCP, explicar la necesidad del visitante autorizado. Los tokens se generan, almacenan mediante proveedor seguro y se referencian por ID; el valor solo se revela mediante acción explícita con tiempo limitado.

## 18.3 Barra de pestañas: especificación

- Tamaño orientativo: mínimo 8, objetivo 22, máximo 32 columnas por pestaña; son parámetros de diseño inspirados en la investigación de OpenCode, **no restricciones del motor**.
- Contenido: número corto, icono de estado, alias truncado y marcador de actividad. El indicador de estado prevalece sobre el color personalizado.
- La pestaña activa siempre es visible; overflow con contadores izquierda/derecha y menú de pestañas buscable.
- Nueva pestaña `+`; cierre `×` por ratón o atajo; reordenación con teclado y, si se valida, drag and drop.
- Al cerrar: ofrecer **Cerrar vista**, **Desconectar SSH**, **Desacoplar tmux** según capacidades. Confirmar acciones destructivas o con procesos activos, con preferencia configurable.
- Reabrir pestaña cerrada restaura el perfil/vista; **no promete restaurar un proceso remoto** sin supervisor persistente o tmux.
- Los colores de pestaña pueden representar host/grupo y ser personalizados; los estados se muestran mediante iconos y texto accesible.

## 18.4 Sistema visual

| Token | Uso | Comportamiento |
|---|---|---|
| `surface.base` | Fondo principal | Tema oscuro/claro |
| `surface.panel` | Paneles e inspector | Contraste legible |
| `text.primary` | Texto operativo | Alto contraste |
| `text.muted` | Metadatos | Nunca único portador de alerta |
| `accent` | Selección/foco | Configurable, no confundir con estado |
| `state.connected` | Conectado | `●` + «Conectado» |
| `state.reconnecting` | Reconectando | `↻` + «Reconectando» |
| `state.warning` | Acción requerida | `!` + descripción |
| `state.failed` | Error | `×` + motivo |

Soportar True Color cuando esté disponible, con fallback 256/16 colores. Respetar `NO_COLOR` cuando proceda y ofrecer tema monocromo. Contraste mínimo objetivo WCAG 2.2 AA para texto donde sea aplicable, teniendo en cuenta que la terminal anfitriona controla la representación final. No usar animaciones constantes en terminales remotas lentas; ofrecer modo movimiento reducido.

## 18.5 Responsive por anchura

| Anchura disponible | Comportamiento |
|---|---|
| ≥120 columnas | Panel de hosts + terminal + inspector opcional |
| 80–119 | Panel de hosts plegable; terminal prioritaria |
| 60–79 | Solo pestañas compactas + terminal; paneles en overlay |
| <60 | Modo mínimo: una terminal, menú de pestañas y estado compacto; avisar si la aplicación remota requiere más columnas |

Altura reducida: minimizar cabeceras antes que recortar el viewport; el redimensionado de PTY debe corresponder a las celdas reales disponibles, nunca al tamaño total de la ventana.

## 18.6 Mapa de teclado propuesto

| Acción | Atajo propuesto | Observación |
|---|---|---|
| Paleta de comandos | `Ctrl+P` | Configurable por conflictos con programas remotos |
| Siguiente/anterior pestaña | `Ctrl+Tab` / `Ctrl+Shift+Tab` | Validar compatibilidad del emulador anfitrión |
| Nueva pestaña | `Ctrl+Shift+T` | Validar contra atajos del emulador anfitrión |
| Cerrar pestaña | `Ctrl+Shift+W` | Nunca implica cierre silencioso de tmux/FRP |
| Buscar host | `/` fuera del terminal | No interceptar `/` con foco terminal |
| Ayuda | `F1` | Disponible desde todas las pantallas |
| Acciones de pestaña | Paleta o prefijo configurable | Fallback cuando el terminal anfitrión captura combinaciones |

**No se garantizan atajos idénticos en todos los emuladores:** `Ctrl+Tab`, `Ctrl+Shift+T` y `Ctrl+Shift+W` pueden ser interceptados por el terminal externo. El SDD exige atajos alternativos basados en un prefijo configurable y pruebas en los terminales objetivo.

## 18.7 Estados vacíos, errores y microinteracciones

- **Sin hosts:** «No hay servidores todavía» y acciones «Importar ssh_config», «Vincular KeePass» y «Añadir host».
- **KeePass bloqueado:** mostrar número de perfiles cacheados no secretos, estado bloqueado y acción para desbloquear; nunca pedir contraseña en una caja de texto genérica sin revisar el proveedor.
- **FRP desconectado:** separar «servidor frps no accesible», «token rechazado», «proxy mal configurado» y «cliente frpc detenido»; proponer acciones específicas.
- **Host key modificada:** diálogo bloqueante con huella anterior/nueva, fuente y opciones seguras; no aceptar automáticamente ni por timeout.
- **Red inestable:** mostrar reconexión y próximo intento, con cancelar; la terminal conserva la última pantalla como contexto visual, marcada como desconectada.
- **Sesión tmux recuperada:** informar «Reenganchada a tmux»; distinguir de «SSH reconectado».
- **Actualización FRP:** previsualización de versión, plataforma, checksum, servicio afectado y reversión antes de aplicar.

## 18.8 Accesibilidad y ergonomía

Navegación completa por teclado, foco visible, mensajes legibles sin color, soporte de alias largos y unicode, textos claros en errores, sin dependencia exclusiva del hover. La TUI debe permitir seleccionar/copiar texto del terminal sin confundirlo con selección de la barra de pestañas. El usuario puede configurar densidad, ancho del panel lateral, posición de pestañas, orden y persistencia de preferencias.

---

# 19. Escenarios de usuario y pruebas UX

## 19.1 Recorrido principal: host privado por FRP

1. El usuario abre la paleta y busca «lab-arm64».
2. ssh05 muestra fuentes del perfil, ruta preferida y verificación de identidad conocida.
3. El motor comprueba si existe ruta directa/ProxyJump autorizada; si no, identifica el FRP asociado.
4. Si el túnel está detenido, muestra «Se necesita iniciar túnel STCP» con alcance, servidor y confirmación.
5. El motor arranca el túnel autorizado, comprueba el estado y solo entonces abre SSH.
6. La pestaña indica alias, estado y ruta. Si se cierra la pestaña, el túnel sigue según política de ciclo de vida, sin detención implícita.
7. Ante fallo, la UI ofrece diagnóstico y reintento, nunca una ruta pública alternativa no autorizada.

## 19.2 Recorrido: recuperar trabajo

1. El usuario reabre ssh05 y elige el workspace «Laboratorio».
2. Aparecen pestañas guardadas con estado «pendiente de restaurar», sin afirmar conexión activa.
3. El backend identifica sesiones persistentes y tmux remotos existentes.
4. La UI distingue «reenganchar sesión del supervisor», «reconectar y adjuntar tmux» y «abrir sesión nueva».
5. Si no existe tmux, no promete recuperar procesos remotos; ofrece nueva conexión.

## 19.3 Recorrido: importar KeePass

1. Se desbloquea un KDBX autorizado mediante un flujo seguro.
2. Se detectan candidatos SSH por URL, host, usuario y puerto; se marcan inferencias y entradas incompletas.
3. Una vista de revisión permite importar, corregir o excluir candidatos; la importación no inicia conexiones.
4. Se fusionan duplicados de `ssh_config`, KDBX y manuales conservando procedencia y referencias de credenciales.
5. Una sincronización posterior muestra conflictos y cambios sin sobrescritura silenciosa.

## 19.4 Criterios de aceptación añadidos (UIX)

| ID | Prueba observable |
|---|---|
| UIX-01 | Abrir 10 pestañas; la activa siempre visible, overflow accesible y búsqueda funcional |
| UIX-02 | Abrir `vim`, `htop` y `tmux` simultáneamente, cambiar de pestaña y redimensionar sin corrupción |
| UIX-03 | Cerrar una vista no mata otras sesiones ni túneles compartidos |
| UIX-04 | Atajos no capturados por el terminal anfitrión tienen alternativas funcionales |
| UIX-05 | Con foco terminal, `Escape` y `/` llegan al programa remoto |
| UIX-06 | Con tema monocromo, estados conectando/conectado/error distinguibles sin color |
| UIX-07 | Ancho 80/60 columnas: acciones esenciales disponibles, PTY dimensionado al viewport |
| UIX-08 | El usuario identifica antes de conectar ruta, destino, identidad y método de autenticación |
| UIX-09 | Una exposición TCP pública de FRP exige confirmación informada |
| UIX-10 | Reinicio de UI con supervisor: reenganche sin crear sesión duplicada |
| UIX-11 | Sin supervisor: restauración de vista no se presenta falsamente como sesión persistente |
| UIX-12 | Bloqueo de KeePass y host key cambiada no se sortean por una reconexión automática |
| UIX-13 | 100 MB de salida simulada: colas acotadas, UI responde y no se pierden errores críticos |
| UIX-14 | Arrastre de pestaña, si se implementa, no interfiere con ratón del terminal remoto |

---

# 20. Registro de cambios de la revisión 0.2


- Se incorporó la investigación aportada sobre `EmbeddedTerminalRenderable`, PTY nativo Bun, limitaciones de Solid y comportamiento de pestañas OpenCode.
- Se precisó la transición de prototipo Bun local a supervisor Go persistente sin acoplar la interfaz a un motor SSH concreto.
- Se separaron formalmente pestaña, vista, PTY, conexión SSH, tmux y túnel FRP.
- Se añadió un sistema de diseño UX/UI, bocetos de tres pantallas, comportamiento responsive, accesibilidad, teclado y manejo de errores.
- Se introdujeron recorridos críticos y 14 criterios de aceptación UIX.
- Se mantuvieron como prioridades del producto: SSH multipestaña, inventario unificado, KeePassXC, rutas seguras y FRP vertical; conexiones serie e imágenes permanecen fuera del alcance inicial.
- **Pendiente:** reproducir las pruebas de investigación y ejecutar integración SSH real, no afirmar que el SDD sustituye pruebas de implementación.

---

# 21. Registro de cambios de la revisión 0.3

- Se reunificó la identidad pública del producto bajo el nombre **ssh05**. El
  binario cliente, la biblioteca, el supervisor y la UI comparten una sola
  denominación; el antiguo nombre interno «trss» queda solo como compatibilidad.
- El vendor es **Dinámicas Electrónicas** y el correo de soporte es
  **dinamicas.electronicas@gmail.com**. Ambos aparecen en `--version`, en la
  ayuda del cliente, en la UI y en los metadatos de los paquetes.
- El supervisor pasa a llamarse **`ssh05d`** (`cmd/ssh05d`), con el servicio de
  usuario `ssh05d.service` y el target `make ssh05d`; `make trssd` se conserva
  como alias temporal.
- La UI pasa de `src/trss.ts` a **`src/ssh05.ts`** y muestra «ssh05 ·
  Dinámicas Electrónicas» en la barra de pestañas.
- Rutas nuevas: `~/.config/ssh05`, `~/.local/state/ssh05` y
  `$XDG_RUNTIME_DIR/ssh05/control.sock`. Un directorio `~/.config/trss` o
  `~/.local/state/trss` preexistente se adopta una sola vez mediante renombrado.
- Variables nuevas `SSH05_*`; `TRSS_*` sigue leyéndose como respaldo para no
  romper scripts, servicios ni sesiones de desarrollo.
- El informe de diagnóstico pasa a `ssh05-diagnostics-*.json` y los nombres de
  sesión tmux generados usan el prefijo `ssh05-`.
- Versión del cliente: **0.3.26**. Se mantiene la compatibilidad con
  `~/.tssh.conf`, `TSSH_*` y `~/.tssh/` heredada de `trzsz-ssh`.
- **DAT-01 completado:** `trz`/`tsz` (y Zmodem opcional con `--zmodem`) se
  integran en el flujo de la sesión del supervisor, con `--download-path` para
  las descargas y `make trzsz-smoke` como prueba real. `transfers.start` sigue
  siendo la API tipada de SCP/SFTP.
- **Publicado:** releases `v0.3.18` (reunificación de identidad), `v0.3.19`
  (trzsz/Zmodem en sesiones) y `v0.3.20` (paquete autosuficiente). Notas en
  `docs/releases/`.
- **Paquete 0.3.20:** el `.deb` incluye `ssh05`, `ssh05d`, `ssh05-ui`, el
  runtime Bun 1.4.5 y las dependencias de producción. `ssh05d.service` queda
  habilitado globalmente para usuarios (`/etc/systemd/user/default.target.wants`)
  y `ssh05-ui` arranca el supervisor si el socket no existe; no se requieren
  pasos manuales de `systemctl`.
- **0.3.21:** la unidad ya no fuerza la desactivación de Docker, de modo que
  `--docker` en `ExecStart` incluye contenedores; la UI explica en el estado
  vacío cómo crear hosts y cómo habilitar Docker.
- **0.3.22:** navegación completa con flechas en menús, submenús, listas y
  formularios (`←/↑` anterior, `→/↓` siguiente), con preferencias, transferencias
  y FRP ajustables sin memorizar atajos.
- **0.3.23:** los overlays ocupan el ancho completo, de modo que la barra de
  acciones y los hosts no se transparentan detrás de menús y formularios.
- **0.3.24:** en el TUI clásico `j`/`k` dejan de navegar; solo las flechas
  (además de PageUp/PageDown/Home/End) cambian de fila, y en búsqueda se
  escriben como texto.
- **0.3.25:** pegado con bracketed paste en búsqueda, formularios y PTY de
  ambos frontends, con límites de tamaño y sin saltos de línea en campos
  simples.
- **0.3.26:** el alta de host admite contraseña opcional y la guarda como
  `#!! encPassword` (esquema `enc` heredado), nunca en claro ni en perfiles;
  la sesión la descodifica al conectar.
