import Foundation
import Observation
import SyncAmpKit

/// Polls the local service and publishes state to the UI.
@MainActor @Observable
final class StatusModel {
    var status: Status?
    var reachable = true
    var message: String?
    private(set) var limited = false
    let client = SyncAmpClient()
    private var poller: Task<Void, Never>?
    private var triedSetup = false

    init() {
        limited = !client.hasToken
        poller = Task { [weak self] in
            while !Task.isCancelled {
                await self?.refresh()
                try? await Task.sleep(for: .seconds(5))
            }
        }
    }

    var overall: String { reachable ? (status?.overall ?? "unknown") : "unknown" }

    /// Menu-bar glyph. The bar renders it as a template (monochrome), so state is
    /// carried by the symbol shape; colour lives in the popover.
    var symbol: String {
        switch overall {
        case "ok": return "externaldrive.badge.checkmark"
        case "error": return "externaldrive.badge.xmark"
        case "running": return "arrow.triangle.2.circlepath"
        case "warn": return "exclamationmark.triangle"
        default: return "externaldrive"
        }
    }

    func refresh() async {
        do {
            status = try await client.status()
            reachable = true; message = nil
        } catch {
            reachable = false
            message = (error as? LocalizedError)?.errorDescription ?? error.localizedDescription
            // First launch (or the service was removed): set it up ourselves, once per app run.
            if !triedSetup, ServiceSetup.canSelfInstall {
                triedSetup = true
                message = "Setting up the background service…"
                if let failure = await ServiceSetup.installAndStart() { message = failure }
                else { try? await Task.sleep(for: .seconds(2)); await refresh() }
            }
        }
    }

    func run(_ profile: String) async {
        do { try await client.run(profile: profile) } catch { message = error.localizedDescription }
        await refresh()
    }

    func runAll() async {
        do { try await client.runAll() } catch { message = error.localizedDescription }
        await refresh()
    }
}
