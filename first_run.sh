#!/usr/bin/env bash
set -euo pipefail

write_if_changed() {
	local target="$1"
	local tmp
	tmp="$(mktemp)"
	cat >"$tmp"
	if [[ -f "$target" ]] && cmp -s "$tmp" "$target"; then
		echo "already configured: $target"
		rm -f "$tmp"
		return 0
	fi
	sudo install -D -m 644 "$tmp" "$target"
	rm -f "$tmp"
	echo "wrote $target"
}

install_deps() {
	if ! command -v apt-get >/dev/null 2>&1; then
		return 0
	fi

	if dpkg -s libnlopt0 >/dev/null 2>&1; then
		echo "libnlopt0 already installed"
		return 0
	fi

	# A broken third-party apt repo (e.g. missing GPG key) must not block kiosk setup.
	if ! sudo apt-get update; then
		echo "WARNING: apt-get update failed; trying to install libnlopt0 anyway" >&2
	fi

	if sudo apt-get install -y libnlopt0; then
		echo "installed libnlopt0"
	else
		echo "WARNING: failed to install libnlopt0 — fix apt repos or install manually" >&2
	fi
}

is_linux_gnome() {
	[[ "$(uname -s)" == "Linux" ]] || return 1
	if command -v gsettings >/dev/null 2>&1; then
		return 0
	fi
	[[ -d /etc/gdm3 ]] || [[ -d /etc/gdm ]]
}

configure_kiosk() {
	if ! is_linux_gnome; then
		echo "skipping kiosk setup: not Linux with GNOME/GDM"
		return 0
	fi

	echo "configuring wine cart kiosk (keep screen on, no sleep)..."

	for target in sleep.target suspend.target hibernate.target hybrid-sleep.target; do
		sudo systemctl mask "$target" 2>/dev/null || true
	done

	sudo mkdir -p /etc/systemd/logind.conf.d
	write_if_changed /etc/systemd/logind.conf.d/99-vino-kiosk.conf <<'EOF'
[Login]
IdleAction=ignore
IdleActionSec=0
HandleLidSwitch=ignore
HandleLidSwitchExternalPower=ignore
HandleLidSwitchDocked=ignore
EOF
	sudo systemctl restart systemd-logind || true

	sudo mkdir -p /etc/dconf/db/local.d
	write_if_changed /etc/dconf/db/local.d/01-vino-kiosk <<'EOF'
[org/gnome/desktop/session]
idle-delay=uint32 0

[org/gnome/desktop/screensaver]
lock-enabled=false
lock-delay=uint32 0

[org/gnome/settings-daemon/plugins/power]
sleep-inactive-ac-type='nothing'
sleep-inactive-battery-type='nothing'
sleep-inactive-ac-timeout=0
sleep-inactive-battery-timeout=0
idle-dim=false
EOF

	sudo mkdir -p /etc/dconf/profile
	write_if_changed /etc/dconf/profile/user <<'EOF'
user-db:user
system-db:local
EOF

	write_if_changed /etc/dconf/profile/gdm <<'EOF'
user-db:gdm
system-db:local
EOF

	sudo dconf update

	if id gdm &>/dev/null; then
		sudo -u gdm dbus-run-session -- gsettings set org.gnome.desktop.session idle-delay 0 || true
		sudo -u gdm dbus-run-session -- gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-ac-type 'nothing' || true
		sudo -u gdm dbus-run-session -- gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-battery-type 'nothing' || true
	fi

	if [[ -f /etc/default/grub ]]; then
		if grep -q 'consoleblank=0' /etc/default/grub; then
			echo "GRUB already has consoleblank=0"
		else
			sudo sed -i '/^GRUB_CMDLINE_LINUX_DEFAULT=/ s/"\(.*\)"/"\1 consoleblank=0"/' /etc/default/grub
			if command -v update-grub >/dev/null 2>&1; then
				sudo update-grub
			elif command -v grub-mkconfig >/dev/null 2>&1; then
				sudo grub-mkconfig -o /boot/grub/grub.cfg
			fi
			echo "NOTE: reboot may be required for GRUB consoleblank=0 to take effect"
		fi
	fi

	echo "kiosk setup complete"
}

configure_kiosk
install_deps
