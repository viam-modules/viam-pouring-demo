#!/usr/bin/env bash
set -euo pipefail

# Set to 1 by write_if_changed when a file was actually written.
CHANGED=0

write_if_changed() {
	local target="$1"
	local tmp
	tmp="$(mktemp)"
	cat >"$tmp"
	if [[ -f "$target" ]] && cmp -s "$tmp" "$target"; then
		echo "already configured: $target"
		rm -f "$tmp"
		CHANGED=0
		return 0
	fi
	sudo install -D -m 644 "$tmp" "$target"
	rm -f "$tmp"
	echo "wrote $target"
	CHANGED=1
	return 0
}

apt_update() {
	if ! command -v apt-get >/dev/null 2>&1; then
		return 1
	fi
	if ! sudo apt-get update; then
		echo "WARNING: apt-get update failed" >&2
		return 1
	fi
	return 0
}

is_gdm_installed() {
	dpkg -s gdm3 >/dev/null 2>&1 || dpkg -s gdm >/dev/null 2>&1
}

gdm_service_name() {
	local unit
	for unit in gdm gdm3; do
		if systemctl list-unit-files "${unit}.service" 2>/dev/null | grep -q "^${unit}.service"; then
			echo "$unit"
			return 0
		fi
	done
	return 1
}

# Enable graphical login if needed. Never restart GDM — that kills the demo UI
# when first_run re-runs on every hot-reload package version.
enable_graphical_login() {
	local gdm_service
	if ! gdm_service="$(gdm_service_name)"; then
		echo "WARNING: GDM package is installed but no gdm systemd unit was found" >&2
		return 1
	fi

	local need_enable=0
	if ! systemctl is-enabled "$gdm_service" >/dev/null 2>&1; then
		need_enable=1
	fi
	if [[ "$(systemctl get-default 2>/dev/null || true)" != "graphical.target" ]]; then
		need_enable=1
	fi

	if [[ "$need_enable" -eq 1 ]]; then
		echo "enabling graphical login via ${gdm_service}.service"
		sudo systemctl enable "$gdm_service"
		sudo systemctl set-default graphical.target
	else
		echo "graphical login already enabled (${gdm_service})"
	fi

	if systemctl is-active --quiet "$gdm_service"; then
		echo "${gdm_service} already active — not restarting"
	else
		echo "starting ${gdm_service}"
		sudo systemctl start "$gdm_service" || true
	fi
}

ensure_gdm() {
	if [[ "$(uname -s)" != "Linux" ]]; then
		echo "skipping GDM setup: not Linux"
		return 0
	fi

	if is_gdm_installed; then
		echo "GDM is installed"
		enable_graphical_login || true
		return 0
	fi

	if ! command -v apt-get >/dev/null 2>&1; then
		echo "WARNING: GDM is not installed and apt-get is unavailable" >&2
		return 0
	fi

	echo "GDM is not installed; installing ubuntu-desktop and gdm3..."
	apt_update || true
	if sudo DEBIAN_FRONTEND=noninteractive apt-get install -y ubuntu-desktop gdm3; then
		enable_graphical_login || true
		echo "NOTE: reboot required after installing GDM/ubuntu-desktop"
	else
		echo "WARNING: failed to install ubuntu-desktop/gdm3" >&2
	fi
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
	apt_update || true

	if sudo apt-get install -y libnlopt0; then
		echo "installed libnlopt0"
	else
		echo "WARNING: failed to install libnlopt0 — fix apt repos or install manually" >&2
	fi
}

is_linux_gnome() {
	[[ "$(uname -s)" == "Linux" ]] || return 1
	if is_gdm_installed; then
		return 0
	fi
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

	local logind_changed=0
	sudo mkdir -p /etc/systemd/logind.conf.d
	write_if_changed /etc/systemd/logind.conf.d/99-vino-kiosk.conf <<'EOF'
[Login]
IdleAction=ignore
IdleActionSec=0
HandleLidSwitch=ignore
HandleLidSwitchExternalPower=ignore
HandleLidSwitchDocked=ignore
EOF
	logind_changed=$CHANGED

	# Restarting systemd-logind tears down the active graphical session. Only do
	# it when the drop-in actually changed (first install / config update).
	if [[ "$logind_changed" -eq 1 ]]; then
		echo "logind kiosk config changed — restarting systemd-logind"
		sudo systemctl restart systemd-logind || true
	else
		echo "skipping systemd-logind restart (config unchanged)"
	fi

	local dconf_changed=0
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
	[[ "$CHANGED" -eq 1 ]] && dconf_changed=1

	sudo mkdir -p /etc/dconf/profile
	write_if_changed /etc/dconf/profile/user <<'EOF'
user-db:user
system-db:local
EOF
	[[ "$CHANGED" -eq 1 ]] && dconf_changed=1

	write_if_changed /etc/dconf/profile/gdm <<'EOF'
user-db:gdm
system-db:local
EOF
	[[ "$CHANGED" -eq 1 ]] && dconf_changed=1

	if [[ "$dconf_changed" -eq 1 ]]; then
		sudo dconf update
		if id gdm &>/dev/null; then
			sudo -u gdm dbus-run-session -- gsettings set org.gnome.desktop.session idle-delay 0 || true
			sudo -u gdm dbus-run-session -- gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-ac-type 'nothing' || true
			sudo -u gdm dbus-run-session -- gsettings set org.gnome.settings-daemon.plugins.power sleep-inactive-battery-type 'nothing' || true
		fi
	else
		echo "skipping dconf update (kiosk settings unchanged)"
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

# Suppress OS update prompts and Chrome update/reminder UI so demos stay clean.
suppress_popups() {
	if [[ "$(uname -s)" != "Linux" ]]; then
		echo "skipping popup suppression: not Linux"
		return 0
	fi

	echo "suppressing OS and Chrome update popups..."

	local dconf_changed=0

	# Stop apt from scheduling automatic update checks / unattended upgrades.
	sudo mkdir -p /etc/apt/apt.conf.d
	write_if_changed /etc/apt/apt.conf.d/99vino-kiosk-no-auto-upgrades <<'EOF'
APT::Periodic::Update-Package-Lists "0";
APT::Periodic::Download-Upgradeable-Packages "0";
APT::Periodic::Unattended-Upgrade "0";
APT::Periodic::AutocleanInterval "0";
EOF

	# Hide autostart notifiers (system-wide overrides).
	sudo mkdir -p /etc/xdg/autostart
	write_if_changed /etc/xdg/autostart/update-notifier.desktop <<'EOF'
[Desktop Entry]
Hidden=true
EOF
	write_if_changed /etc/xdg/autostart/gnome-software-service.desktop <<'EOF'
[Desktop Entry]
Hidden=true
EOF

	# GNOME Software + Ubuntu update-notifier dconf.
	sudo mkdir -p /etc/dconf/db/local.d /etc/dconf/profile
	write_if_changed /etc/dconf/db/local.d/02-vino-no-popups <<'EOF'
[org/gnome/software]
allow-updates=false
download-updates=false

[com/ubuntu/update-notifier]
no-show-notifications=true
show-livepatch-status-icon=false
EOF
	[[ "$CHANGED" -eq 1 ]] && dconf_changed=1

	# Ensure user profile includes system-db (may already be set by configure_kiosk).
	write_if_changed /etc/dconf/profile/user <<'EOF'
user-db:user
system-db:local
EOF
	[[ "$CHANGED" -eq 1 ]] && dconf_changed=1

	if [[ "$dconf_changed" -eq 1 ]]; then
		sudo dconf update || true
	fi

	# Disable timers/services that surface update UI (best-effort; ignore missing units).
	local unit
	for unit in \
		update-notifier-download.timer \
		update-notifier-motd.timer \
		apt-daily.timer \
		apt-daily-upgrade.timer \
		unattended-upgrades.service \
		packagekit.service; do
		sudo systemctl disable --now "$unit" 2>/dev/null || true
		sudo systemctl mask "$unit" 2>/dev/null || true
	done

	# Chrome managed policies: no relaunch nags, no component/promo noise.
	sudo mkdir -p /etc/opt/chrome/policies/managed
	write_if_changed /etc/opt/chrome/policies/managed/vino-kiosk.json <<'EOF'
{
  "ComponentUpdatesEnabled": false,
  "RelaunchNotification": 0,
  "PromotionalTabsEnabled": false,
  "DefaultBrowserSettingEnabled": false,
  "BrowserSignin": 0,
  "SyncDisabled": true,
  "TranslateEnabled": false,
  "PasswordManagerEnabled": false,
  "AutofillAddressEnabled": false,
  "AutofillCreditCardEnabled": false,
  "SuppressUnsupportedOSWarning": true,
  "CommandLineFlagSecurityWarningsEnabled": false
}
EOF

	# Stop Google's postinst from (re)enabling the Chrome apt repo on this machine.
	write_if_changed /etc/default/google-chrome <<'EOF'
repo_add_once=false
repo_reenable_on_close=false
EOF

	# Hold Chrome packages so update-manager has nothing to offer for the browser.
	if dpkg -s google-chrome-stable >/dev/null 2>&1; then
		sudo apt-mark hold google-chrome-stable >/dev/null 2>&1 || true
		echo "held google-chrome-stable"
	fi
	if dpkg -s chromium-browser >/dev/null 2>&1; then
		sudo apt-mark hold chromium-browser >/dev/null 2>&1 || true
		echo "held chromium-browser"
	fi
	if dpkg -s chromium >/dev/null 2>&1; then
		sudo apt-mark hold chromium >/dev/null 2>&1 || true
		echo "held chromium"
	fi

	echo "popup suppression complete"
}

ensure_gdm
configure_kiosk
suppress_popups
install_deps
