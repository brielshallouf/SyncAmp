package main

// hardenedUnit replaces kardianos/service's default unit with one that
// sandboxes the service. It deliberately does NOT set ProtectHome or
// CapabilityBoundingSet: backups must read (and the Syncrify client writes
// state under) user homes, and root needs CAP_DAC_READ_SEARCH/CAP_SETUID to
// back up files and drop to each profile's owner. MemoryDenyWriteExecute is
// off because the Syncrify client is Java (JIT).
const hardenedUnit = `[Unit]
Description={{Description}}
ConditionFileIsExecutable={{Path | cmdEscape}}
After=network-online.target
Wants=network-online.target

[Service]
ExecStart={{Path | cmdEscape}}{{range Arguments}} {{. | cmd}}{{end}}
Restart=on-failure
RestartSec=10
StartLimitIntervalSec=300
StartLimitBurst=10
UMask=0077
NoNewPrivileges=yes
PrivateTmp=yes
ProtectSystem=full
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectKernelLogs=yes
ProtectControlGroups=yes
ProtectClock=yes
ProtectHostname=yes
RestrictSUIDSGID=yes
RestrictRealtime=yes
RestrictNamespaces=yes
LockPersonality=yes
SystemCallArchitectures=native
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6

[Install]
WantedBy=multi-user.target
`
