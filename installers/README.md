# Instaladores ssh05 0.3.13

Los `.deb` para Ubuntu y Debian están en esta carpeta:

```text
Incluye los binarios ssh05 y trssd.

ssh05_0.3.13_ubuntu_amd64.deb
ssh05_0.3.13_ubuntu_arm64.deb
ssh05_0.3.13_debian_amd64.deb
ssh05_0.3.13_debian_arm64.deb
*.sha256
```

Incluyen el cliente `ssh05` y el supervisor `trssd`.

Instalación:

```sh
sudo apt install ./Incluye los binarios ssh05 y trssd.

ssh05_0.3.13_ubuntu_amd64.deb
```

También están publicados en:

https://github.com/lordpietre/ssh05/releases/tag/v0.3.13

Para regenerarlos:

```sh
make deb
ls dist/*.deb
```
