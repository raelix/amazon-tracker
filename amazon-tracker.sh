#!/bin/sh
# Gestione di Amazon Sniper Bot come LaunchAgent macOS.
# Uso: ./amazon-tracker.sh {build|enable|disable|start|stop|restart|status|logs}

set -eu

LABEL="com.raelix.amazon-tracker"
DIR="$(cd "$(dirname "$0")" && pwd)"
BIN="$DIR/sniper-mac"
LOG="$HOME/Library/Logs/amazon-tracker.log"
SUPPORT_DIR="$HOME/Library/Application Support/amazon-tracker"
PLIST="$SUPPORT_DIR/$LABEL.plist"
BOOT_PLIST="$HOME/Library/LaunchAgents/$LABEL.plist"
DOMAIN="gui/$(id -u)"

write_plist() {
	mkdir -p "$SUPPORT_DIR"
	cat > "$PLIST" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>$LABEL</string>
	<key>ProgramArguments</key>
	<array>
		<string>$BIN</string>
	</array>
	<key>WorkingDirectory</key>
	<string>$DIR</string>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>30</integer>
	<key>StandardOutPath</key>
	<string>$LOG</string>
	<key>StandardErrorPath</key>
	<string>$LOG</string>
</dict>
</plist>
EOF
}

is_loaded() {
	launchctl print "$DOMAIN/$LABEL" >/dev/null 2>&1
}

build() {
	echo "Compilo $BIN..."
	(cd "$DIR" && go build -o "$BIN" ./cmd/sniper)
}

start() {
	[ -x "$BIN" ] || build
	if is_loaded; then
		echo "Già in esecuzione."
		return
	fi
	write_plist
	launchctl bootstrap "$DOMAIN" "$PLIST"
	echo "Avviato. Log: $LOG"
}

stop() {
	if is_loaded; then
		launchctl bootout "$DOMAIN/$LABEL"
		# bootout is asynchronous: wait until the service is really gone.
		i=0
		while is_loaded && [ "$i" -lt 20 ]; do
			sleep 0.5
			i=$((i + 1))
		done
		echo "Fermato."
	else
		echo "Non in esecuzione."
	fi
}

enable() {
	write_plist
	mkdir -p "$(dirname "$BOOT_PLIST")"
	cp "$PLIST" "$BOOT_PLIST"
	echo "Abilitato all'avvio (al login dell'utente)."
}

disable() {
	rm -f "$BOOT_PLIST"
	echo "Disabilitato all'avvio. Lo stato attuale non cambia (usa 'stop' per fermarlo)."
}

status() {
	if [ -f "$BOOT_PLIST" ]; then boot="abilitato"; else boot="disabilitato"; fi
	if is_loaded; then
		pid="$(launchctl print "$DOMAIN/$LABEL" | awk '/^\tpid = /{print $3}')"
		echo "Stato: in esecuzione${pid:+ (pid $pid)} — avvio al boot: $boot"
	else
		echo "Stato: fermo — avvio al boot: $boot"
	fi
}

case "${1:-}" in
	build)   build ;;
	enable)  enable ;;
	disable) disable ;;
	start)   start ;;
	stop)    stop ;;
	restart) stop; start ;;
	status)  status ;;
	logs)    tail -f "$LOG" ;;
	*)
		echo "Uso: $0 {build|enable|disable|start|stop|restart|status|logs}" >&2
		exit 1
		;;
esac
