import Foundation

/// SyncAmp.app carries its own background service (Contents/Resources/syncamp), so there is
/// ONE thing to download and nothing to install by hand. On launch, if the service is not
/// answering, the app (re)registers it as the user's login item pointing at the copy inside
/// this app, then starts it. Re-registering every time also repairs the path if the app was moved.
enum ServiceSetup {
    static var bundled: URL? { Bundle.main.url(forResource: "syncamp", withExtension: nil) }
    static var canSelfInstall: Bool { bundled != nil }

    /// Runs `syncamp <args>` and returns its exit code (-1 if it could not be launched).
    @discardableResult
    private static func run(_ bin: URL, _ args: [String]) async -> Int32 {
        await withCheckedContinuation { cont in
            let p = Process()
            p.executableURL = bin
            p.arguments = args
            p.standardOutput = FileHandle.nullDevice
            p.standardError = FileHandle.nullDevice
            p.terminationHandler = { cont.resume(returning: $0.terminationStatus) }
            do { try p.run() } catch { cont.resume(returning: -1) }
        }
    }

    /// Returns a human message if setup failed, nil on success (or if there is nothing to do).
    static func installAndStart() async -> String? {
        guard let bin = bundled else { return nil }
        await run(bin, ["stop"])        // ignore: may not be installed yet
        await run(bin, ["uninstall"])   // ignore: lets install record this app's current location
        if await run(bin, ["install"]) != 0 { return "Could not set up the background service." }
        if await run(bin, ["start"]) != 0 { return "The background service did not start." }
        return nil
    }
}
