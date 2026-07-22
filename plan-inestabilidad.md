# Plan de diagnóstico y solución para inestabilidad de conexiones

## Problema
Las conexiones SSH se congelan/timeout durante sesiones normales.

## Fase 1: Diagnóstico (recopilar información)

### 1.1 Verificar configuración actual
```bash
# Ver configuración activa de tssh
cat ~/.tssh.conf 2>/dev/null || echo "No existe ~/.tssh.conf"

# Ver configuración SSH del host
cat ~/.ssh/config | grep -A 10 -i "ServerAlive" || echo "No hay configuración de keepalive"

# Ver configuración efectiva de un host específico
tssh -G <host> | grep -i "alive"
```

### 1.2 Habilitar logs de debug
```bash
# Ejecutar con verbose para ver qué está pasando
tssh -vvv <host>

# O habilitar debug logging en ~/.tssh.conf
echo "LogLevel = DEBUG" >> ~/.tssh.conf
```

### 1.3 Monitorear keepalives
Crear script de diagnóstico:
```bash
#!/bin/bash
# save as: /tmp/monitor-keepalive.sh
HOST=$1
LOG=/tmp/ssh-keepalive-$(date +%Y%m%d-%H%M%S).log

echo "Monitoreando keepalives para $HOST..." | tee $LOG
echo "Log guardado en: $LOG" | tee -a $LOG

# Ejecutar tssh con verbose y capturar output
tssh -vvv $HOST 2>&1 | grep -E "(keep.?alive|timeout|disconnect)" | tee -a $LOG
```

### 1.4 Verificar timeouts del sistema
```bash
# Verificar si hay firewalls/NAT que cierren conexiones
sysctl net.ipv4.tcp_keepalive_time
sysctl net.ipv4.tcp_keepalive_intvl
sysctl net.ipv4.tcp_keepalive_probes

# Ver conexiones activas
ss -tnp | grep ssh
```

## Fase 2: Soluciones incrementales

### 2.1 Ajustar keepalive en ~/.tssh.conf
```ini
# Configuración conservadora
DefaultServerAliveInterval = 15
DefaultServerAliveCountMax = 3
```

### 2.2 Configurar por host específico en ~/.ssh/config
```ssh-config
Host problematico
    HostName ejemplo.com
    ServerAliveInterval 15
    ServerAliveCountMax 5
    TCPKeepAlive yes
```

### 2.3 Usar opción de línea de comandos
```bash
tssh -o ServerAliveInterval=15 -o ServerAliveCountMax=5 <host>
```

### 2.4 Habilitar TCP keepalive a nivel de sistema
```bash
# Temporal (hasta reboot)
sudo sysctl net.ipv4.tcp_keepalive_time=60
sudo sysctl net.ipv4.tcp_keepalive_intvl=10
sudo sysctl net.ipv4.tcp_keepalive_probes=6

# Permanente
echo "net.ipv4.tcp_keepalive_time=60" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_keepalive_intvl=10" | sudo tee -a /etc/sysctl.conf
echo "net.ipv4.tcp_keepalive_probes=6" | sudo tee -a /etc/sysctl.conf
sudo sysctl -p
```

## Fase 3: Diagnóstico avanzado

### 3.1 Capturar tráfico de red
```bash
# Capturar paquetes SSH para analizar
sudo tcpdump -i any -w /tmp/ssh-debug.pcap 'port 22' &
tssh <host>
# Después de reproducir el problema:
sudo killall tcpdump
# Analizar con Wireshark o:
tshark -r /tmp/ssh-debug.pcap -Y "ssh" | head -50
```

### 3.2 Verificar si es problema de red o de tssh
```bash
# Probar con ssh estándar
ssh -o ServerAliveInterval=15 -o ServerAliveCountMax=5 <host>

# Si ssh estándar funciona pero tssh no, es problema de tssh
# Si ambos fallan, es problema de red/servidor
```

### 3.3 Monitorear conexión con script
```bash
#!/bin/bash
# save as: /tmp/monitor-connection.sh
HOST=$1
INTERVAL=5
LOG=/tmp/connection-monitor-$(date +%Y%m%d-%H%M%S).log

echo "Monitoreando conexión a $HOST cada ${INTERVAL}s..." | tee $LOG

while true; do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    if nc -z -w 2 $HOST 22 2>/dev/null; then
        echo "$TIMESTAMP - Puerto 22 accesible" | tee -a $LOG
    else
        echo "$TIMESTAMP - Puerto 22 NO accesible" | tee -a $LOG
    fi
    sleep $INTERVAL
done
```

## Fase 4: Soluciones específicas según síntomas

### Si el problema es después de inactividad:
- Aumentar frecuencia de keepalive (15s en vez de 30s)
- Habilitar TCPKeepAlive en ~/.ssh/config
- Verificar firewalls/NAT que cierren conexiones inactivas

### Si el problema es con servidores específicos:
- Configurar ServerAliveInterval más bajo para ese host
- Verificar configuración del servidor SSH (sshd_config)
- Probar con cliente SSH alternativo para descartar problema de servidor

### Si el problema es generalizado:
- Verificar configuración de red local
- Probar con diferentes servidores para aislar el problema
- Considerar problemas de MTU o Quality of Service (QoS)

## Fase 5: Verificación de solución

### Script de validación
```bash
#!/bin/bash
# save as: /tmp/validate-fix.sh
HOST=$1
DURATION=300  # 5 minutos

echo "Probando estabilidad de conexión a $HOST por ${DURATION}s..."
START=$(date +%s)
tssh -o ServerAliveInterval=15 -o ServerAliveCountMax=5 $HOST "sleep $DURATION && echo 'Conexión estable'" &
PID=$!

# Monitorear cada 30 segundos
for i in $(seq 1 $((DURATION/30))); do
    if ps -p $PID > /dev/null 2>&1; then
        echo "$(date '+%H:%M:%S') - Conexión activa ($((i*30))s)"
    else
        echo "$(date '+%H:%M:%S') - Conexión perdida a los $((i*30))s"
        exit 1
    fi
    sleep 30
done

wait $PID
echo "Prueba completada exitosamente"
```

## Checklist de diagnóstico

- [ ] Verificar configuración actual de keepalive
- [ ] Habilitar logs de debug
- [ ] Probar con ssh estándar para comparar
- [ ] Monitorear keepalives durante sesión
- [ ] Verificar timeouts de TCP a nivel de sistema
- [ ] Capturar tráfico si es necesario
- [ ] Aplicar solución incremental
- [ ] Validar con prueba de estabilidad
- [ ] Documentar configuración que funciona

## Configuraciones recomendadas

### Conservadora (para redes inestables):
```ini
# ~/.tssh.conf
DefaultServerAliveInterval = 10
DefaultServerAliveCountMax = 6
```

### Estándar (para la mayoría de casos):
```ini
# ~/.tssh.conf
DefaultServerAliveInterval = 15
DefaultServerAliveCountMax = 3
```

### Agresiva (para conexiones muy inestables):
```ini
# ~/.tssh.conf
DefaultServerAliveInterval = 5
DefaultServerAliveCountMax = 12
```
