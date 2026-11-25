# Instaladores ssh05 0.42.1

Los `.deb` para Ubuntu y Debian están en esta carpeta:

```text
ssh05_0.42.1_ubuntu_amd64.deb
ssh05_0.42.1_ubuntu_arm64.deb
ssh05_0.42.1_debian_amd64.deb
ssh05_0.42.1_debian_arm64.deb
*.sha256
```

Incluyen el cliente `ssh05`, el supervisor `ssh05d` y la UI OpenTUI
`ssh05-ui` con su runtime Bun y dependencias. El supervisor queda habilitado
como unidad de usuario para arrancar al iniciar sesión, sin pasos manuales de
`systemctl`; `ssh05-ui` lo arranca también si aún no está vivo.

```sh
ssh05 usuario@host
ssh05-ui
```

Instalación (verifica antes la suma):

```sh
sha256sum -c ssh05_0.42.1_ubuntu_amd64.deb.sha256
sudo apt install ./ssh05_0.42.1_ubuntu_amd64.deb
```

También se publican en:

https://github.com/lordpietre/ssh05/releases/tag/v0.42.1

Para regenerarlos:

```sh
make deb
ls dist/*.deb
```
