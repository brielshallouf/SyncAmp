import SwiftUI

/// Liquid Glass on macOS 26+, system material with a hairline edge before that.
struct GlassCard: ViewModifier {
    var radius: CGFloat = 20
    func body(content: Content) -> some View {
        #if compiler(>=6.2)
        if #available(macOS 26.0, *) {
            content.glassEffect(.regular, in: .rect(cornerRadius: radius))
        } else {
            legacy(content)
        }
        #else
        legacy(content)
        #endif
    }

    private func legacy(_ content: Content) -> some View {
        content
            .background(.regularMaterial, in: RoundedRectangle(cornerRadius: radius, style: .continuous))
            .overlay(RoundedRectangle(cornerRadius: radius, style: .continuous)
                .strokeBorder(.white.opacity(0.18), lineWidth: 0.5))
    }
}

extension View {
    func glassCard(radius: CGFloat = 20) -> some View { modifier(GlassCard(radius: radius)) }
}

/// One accent per state, matching the web dashboard.
enum Tone {
    static func color(_ state: String) -> Color {
        switch state {
        case "ok", "success": return .green
        case "running": return .blue
        case "warn": return .orange
        case "error", "failed", "timeout": return .red
        default: return .gray
        }
    }
    static func glyph(_ overall: String) -> String {
        switch overall {
        case "ok": return "checkmark"
        case "running": return "arrow.triangle.2.circlepath"
        case "warn": return "exclamationmark"
        case "error": return "xmark"
        default: return "ellipsis"
        }
    }
    static func headline(_ overall: String) -> String {
        switch overall {
        case "ok": return "All backups healthy"
        case "running": return "Backing up…"
        case "warn": return "Needs a look"
        case "error": return "Needs attention"
        default: return "SyncAmp"
        }
    }
}

func relative(_ date: Date?) -> String {
    guard let date else { return "never" }
    if abs(date.timeIntervalSinceNow) < 45 { return "just now" }
    let f = RelativeDateTimeFormatter()
    f.unitsStyle = .short
    return f.localizedString(for: date, relativeTo: Date())
}
