import AppKit
import ServiceManagement
import SwiftUI
import SyncAmpKit

struct PopoverView: View {
    var model: StatusModel
    @State private var launchAtLogin = SMAppService.mainApp.status == .enabled

    var body: some View {
        VStack(spacing: 12) {
            header
            if !model.reachable { unreachable } else { content }
            footer
        }
        .padding(14)
        .frame(width: 340)
    }

    // MARK: header
    private var header: some View {
        let tone = Tone.color(model.overall)
        return HStack(spacing: 14) {
            ZStack {
                Circle().fill(tone.gradient)
                Circle().strokeBorder(.white.opacity(0.35), lineWidth: 1)
                Image(systemName: Tone.glyph(model.overall))
                    .font(.system(size: 20, weight: .bold)).foregroundStyle(.white)
                    .symbolEffect(.pulse, isActive: model.overall == "running") // .rotate needs macOS 15
            }
            .frame(width: 46, height: 46)
            .shadow(color: tone.opacity(0.35), radius: 8, y: 2)
            VStack(alignment: .leading, spacing: 2) {
                Text(model.reachable ? Tone.headline(model.overall) : "Service not running")
                    .font(.system(.title3, design: .rounded, weight: .semibold))
                Text(subtitle).font(.callout).foregroundStyle(.secondary).lineLimit(2)
            }
            Spacer(minLength: 0)
        }
        .padding(14).frame(maxWidth: .infinity, alignment: .leading).glassCard()
    }

    private var subtitle: String {
        guard model.reachable, let s = model.status else { return "Start it with: syncamp start" }
        var parts: [String] = []
        if s.overall == "ok" || s.overall == "running" {
            parts.append(s.lastBackup.map { "Last backup \(relative($0))" } ?? "No backups yet")
        } else { parts.append(s.summary) }
        if let srv = s.server { parts.append(srv.up ? "Server reachable" : "Can't reach server") }
        return parts.joined(separator: " · ")
    }

    // MARK: body
    @ViewBuilder private var content: some View {
        if let s = model.status {
            ForEach(s.findings.prefix(2), id: \.self) { f in notice(f.msg, level: f.level) }
            if s.profiles.isEmpty && !model.limited {
                Text("No Syncrify profiles found").font(.callout).foregroundStyle(.secondary)
                    .frame(maxWidth: .infinity).padding(.vertical, 18).glassCard(radius: 18)
            } else if !s.profiles.isEmpty {
                VStack(spacing: 0) {
                    ForEach(Array(s.profiles.enumerated()), id: \.element.id) { i, p in
                        if i > 0 { Divider().padding(.leading, 36) }
                        row(p)
                    }
                }
                .padding(.vertical, 4).glassCard(radius: 18)
            }
            if model.limited {
                notice("Limited view: run SyncAmp as your user to control backups from here.", level: "INFO")
            }
        }
        if let m = model.message { notice(m, level: "WARN") }
    }

    private func row(_ p: ProfileStatus) -> some View {
        let state = p.running ? "running" : (p.paused ? "paused" : (p.last?.status ?? "unknown"))
        return HStack(spacing: 10) {
            Circle().fill(p.paused ? Color.clear : Tone.color(state)).frame(width: 9, height: 9)
                .overlay(Circle().strokeBorder(Color.gray, lineWidth: p.paused ? 1.5 : 0))
            VStack(alignment: .leading, spacing: 1) {
                Text(p.name).font(.system(.body, weight: .medium)).lineLimit(1)
                Text(caption(p)).font(.caption)
                    .foregroundStyle(p.last?.status == "failed" && !p.running ? Color.red : Color.secondary)
                    .lineLimit(2)
            }
            Spacer(minLength: 4)
            Button { Task { await model.run(p.name) } } label: {
                Image(systemName: "play.fill").font(.system(size: 11, weight: .bold))
                    .frame(width: 28, height: 28).background(.thinMaterial, in: Circle())
            }
            .buttonStyle(.plain).disabled(p.running || model.limited)
            .help("Back up now").accessibilityLabel("Back up \(p.name) now")
        }
        .padding(.horizontal, 14).padding(.vertical, 9)
    }

    private func caption(_ p: ProfileStatus) -> String {
        if p.running { return "Backing up now…" }
        if p.paused { return "Paused" }
        guard let l = p.last else { return p.scheduled ? "Waiting for first run" : "Manual · never run" }
        switch l.status {
        case "success": return "Succeeded \(relative(l.end))"
        case "warning": return "Finished with warnings \(relative(l.end))"
        case "timeout": return "Timed out \(relative(l.end))"
        default: return "Failed \(relative(l.end))" + (l.error.map { " · \($0)" } ?? "")
        }
    }

    private func notice(_ text: String, level: String) -> some View {
        Text(text).font(.caption).foregroundStyle(.primary)
            .padding(10).frame(maxWidth: .infinity, alignment: .leading)
            .background((level == "FAIL" ? Color.red : level == "INFO" ? Color.blue : Color.orange).opacity(0.15),
                        in: RoundedRectangle(cornerRadius: 12, style: .continuous))
    }

    private var unreachable: some View {
        VStack(spacing: 6) {
            Text(model.message ?? "The SyncAmp service isn't running.").font(.callout)
            Text("Install it with “syncamp install”, then “syncamp start”.")
                .font(.caption).foregroundStyle(.secondary)
        }
        .multilineTextAlignment(.center).padding(16).frame(maxWidth: .infinity).glassCard(radius: 18)
    }

    // MARK: footer
    private var footer: some View {
        HStack(spacing: 8) {
            Button { Task { await model.runAll() } } label: { Text("Back up all").fontWeight(.semibold) }
                .buttonStyle(.borderedProminent).controlSize(.regular).clipShape(Capsule())
                .disabled(!model.reachable || model.limited || (model.status?.profiles.isEmpty ?? true))
            Spacer()
            Menu {
                Button("Open dashboard") { NSWorkspace.shared.open(model.client.base) }
                Toggle("Launch at login", isOn: Binding(get: { launchAtLogin }, set: setLaunchAtLogin))
                Divider()
                Button("Quit SyncAmp") { NSApplication.shared.terminate(nil) }.keyboardShortcut("q")
            } label: {
                Image(systemName: "ellipsis").frame(width: 28, height: 28)
                    .background(.thinMaterial, in: Circle())
            }
            .menuStyle(.button).buttonStyle(.plain).menuIndicator(.hidden).fixedSize()
        }
    }

    private func setLaunchAtLogin(_ on: Bool) {
        do {
            if on { try SMAppService.mainApp.register() } else { try SMAppService.mainApp.unregister() }
        } catch { model.message = "Launch at login: \(error.localizedDescription)" }
        launchAtLogin = SMAppService.mainApp.status == .enabled
    }
}
