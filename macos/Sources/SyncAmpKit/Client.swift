import Foundation

public enum SyncAmpError: Error, LocalizedError {
    case unreachable, unauthorized, http(Int, String)
    public var errorDescription: String? {
        switch self {
        case .unreachable: return "The SyncAmp service isn't running."
        case .unauthorized: return "The API token was rejected."
        case .http(let code, let msg): return "Service error \(code): \(msg)"
        }
    }
}

/// Talks to the SyncAmp service on loopback. The service runs as a per-user
/// LaunchAgent, so the API token in the user's own Application Support folder
/// is readable by this app (and by nobody else).
public final class SyncAmpClient: @unchecked Sendable {
    public let base: URL
    private let token: String?
    private let session: URLSession

    public var hasToken: Bool { token != nil }

    public static func defaultDataDir() -> URL {
        FileManager.default.homeDirectoryForCurrentUser
            .appendingPathComponent("Library/Application Support/SyncAmp", isDirectory: true)
    }

    public init(dataDir: URL = SyncAmpClient.defaultDataDir()) {
        var addr = "127.0.0.1:5801"
        if let data = try? Data(contentsOf: dataDir.appendingPathComponent("config.json")),
           let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
           let a = obj["listenAddr"] as? String, !a.isEmpty {
            addr = a
        }
        self.base = URL(string: "http://\(addr)") ?? URL(string: "http://127.0.0.1:5801")!
        let raw = try? String(contentsOf: dataDir.appendingPathComponent("api.token"), encoding: .utf8)
        let t = raw?.trimmingCharacters(in: .whitespacesAndNewlines)
        self.token = (t?.count ?? 0) >= 32 ? t : nil
        let cfg = URLSessionConfiguration.ephemeral
        cfg.timeoutIntervalForRequest = 5
        cfg.waitsForConnectivity = false
        self.session = URLSession(configuration: cfg)
    }

    private func request(_ path: String, method: String = "GET", auth: Bool = true) async throws -> Data {
        var req = URLRequest(url: base.appendingPathComponent(path))
        req.httpMethod = method
        if auth, let token { req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization") }
        let result: (Data, URLResponse)
        do { result = try await session.data(for: req) } catch { throw SyncAmpError.unreachable }
        let (data, resp) = result
        guard let http = resp as? HTTPURLResponse else { throw SyncAmpError.unreachable }
        switch http.statusCode {
        case 200..<300: return data
        case 401: throw SyncAmpError.unauthorized
        default:
            let msg = (try? JSONSerialization.jsonObject(with: data) as? [String: Any])?["error"] as? String
            throw SyncAmpError.http(http.statusCode, msg ?? "request failed")
        }
    }

    /// Full status when a token is available, otherwise the public summary.
    public func status() async throws -> Status {
        if hasToken {
            return try JSONDecoder().decode(Status.self, from: try await request("/api/status"))
        }
        let d = try await request("/api/summary", auth: false)
        let o = (try JSONSerialization.jsonObject(with: d) as? [String: Any]) ?? [:]
        return Status(overall: o["overall"] as? String ?? "unknown", summary: o["summary"] as? String ?? "")
    }

    public func run(profile: String) async throws {
        let enc = profile.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? profile
        _ = try await request("/api/run/\(enc)", method: "POST")
    }

    public func runAll() async throws { _ = try await request("/api/run", method: "POST") }
}
