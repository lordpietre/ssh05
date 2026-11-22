# Instaladores ssh05 0.3.16

Los `.deb` para Ubuntu y Debian están en esta carpeta:

```text
ssh05_0.3.16_ubuntu_amd64.deb
ssh05_0.3.16_ubuntu_arm64.deb
ssh05_0.3.16_debian_amd64.deb
ssh05_0.3.16_debian_arm64.deb
*.sha256
```

Incluyen el cliente `ssh05` y el supervisor `trssd`.

La UI OpenTUI no está dentro del `.deb`; requiere Bun y se ejecuta desde el
repositorio (`prototype/terminal`, `bun install && bun run ui`).

El paquete instala `trssd.service`:

```sh
systemctl --user daemon-reload
systemctl --user enable --now trssd
```

Instalación:

```sh
sudo apt install ./ssh05_0.3.16_ubuntu_amd64.deb
```

También están publicados en:

https://github.com/lordpietre/ssh05/releases/tag/v0.3.16

Para regenerarlos:

```sh
make deb
ls dist/*.deb
```
