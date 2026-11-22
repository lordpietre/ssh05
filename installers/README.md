# Instaladores ssh05 0.3.13

Los `.deb` para Ubuntu y Debian están en esta carpeta:

```text
ssh05_0.3.13_ubuntu_amd64.deb
ssh05_0.3.13_ubuntu_arm64.deb
ssh05_0.3.13_debian_amd64.deb
ssh05_0.3.13_debian_arm64.deb
*.sha256
```

Instalación:

```sh
sudo apt install ./ssh05_0.3.13_ubuntu_amd64.deb
```

También están publicados en:

https://github.com/lordpietre/ssh05/releases/tag/v0.3.13

Para regenerarlos:

```sh
make deb
ls dist/*.deb
```
