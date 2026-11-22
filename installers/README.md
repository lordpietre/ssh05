# Instaladores ssh05 0.3.19

Los `.deb` para Ubuntu y Debian están en esta carpeta:

```text
ssh05_0.3.19_ubuntu_amd64.deb
ssh05_0.3.19_ubuntu_arm64.deb
ssh05_0.3.19_debian_amd64.deb
ssh05_0.3.19_debian_arm64.deb
*.sha256
```

Incluyen el cliente `ssh05` y el supervisor `ssh05d`.

La UI OpenTUI no está dentro del `.deb`; requiere Bun y se ejecuta desde el
repositorio (`prototype/terminal`, `bun install && bun run ui`).

El paquete instala `ssh05d.service`:

```sh
systemctl --user daemon-reload
systemctl --user enable --now ssh05d
```

Instalación (verifica antes la suma):

```sh
sha256sum -c ssh05_0.3.19_ubuntu_amd64.deb.sha256
sudo apt install ./ssh05_0.3.19_ubuntu_amd64.deb
```

También se publican en:

https://github.com/lordpietre/ssh05/releases/tag/v0.3.19

Para regenerarlos:

```sh
make deb
ls dist/*.deb
```
