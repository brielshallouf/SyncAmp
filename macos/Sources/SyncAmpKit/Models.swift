import Foundation

/// Mirrors the JSON served by the SyncAmp service (`/api/status`, `/api/summary`).

public struct Finding: Decodable, Sendable, Hashable {
    public let level: String
    public let msg: String
}

public struct SkippedProfile: Decodable, Sendable, Hashable {
    public let path: String
    public let reason: String
}

public struct ServerHealth: Decodable, Sendable {
    public let url: String
    public let up: Bool
    public let identified: Bool?
    public let build: String?
    public let error: String?
}

public struct Run: Decodable, Sendable {
    public let profile: String
    public let status: String          // success | failed | timeout
    public let trigger: String?
    public let error: String?
    public let end: Date?

    enum CodingKeys: String, CodingKey { case profile, status, trigger, error, end }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        profile = try c.decode(String.self, forKey: .profile)
        status = try c.decode(String.self, forKey: .status)
        trigger = try c.decodeIfPresent(String.self, forKey: .trigger)
        error = try c.decodeIfPresent(String.self, forKey: .error)
        end = GoTime.parse(try c.decodeIfPresent(String.self, forKey: .end))
    }
}

public struct ProfileStatus: Decodable, Sendable, Identifiable {
    public let name: String
    public let running: Bool
    public let scheduled: Bool
    public let paused: Bool
    public let overdue: Bool
    public let lastSuccess: Date?
    public let last: Run?
    public var id: String { name }

    enum CodingKeys: String, CodingKey { case name, running, scheduled, paused, overdue, lastSuccess, last }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        name = try c.decode(String.self, forKey: .name)
        running = try c.decode(Bool.self, forKey: .running)
        scheduled = try c.decode(Bool.self, forKey: .scheduled)
        paused = try c.decodeIfPresent(Bool.self, forKey: .paused) ?? false
        overdue = try c.decodeIfPresent(Bool.self, forKey: .overdue) ?? false
        lastSuccess = GoTime.parse(try c.decodeIfPresent(String.self, forKey: .lastSuccess))
        last = try c.decodeIfPresent(Run.self, forKey: .last)
    }
}

public struct Status: Decodable, Sendable {
    public let overall: String         // ok | running | warn | error | idle
    public let summary: String
    public let uptime: String?
    public let server: ServerHealth?
    public let profiles: [ProfileStatus]
    public let findings: [Finding]
    public let skippedProfiles: [SkippedProfile]

    enum CodingKeys: String, CodingKey { case overall, summary, uptime, server, profiles, findings, skippedProfiles }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        overall = try c.decode(String.self, forKey: .overall)
        summary = try c.decode(String.self, forKey: .summary)
        uptime = try c.decodeIfPresent(String.self, forKey: .uptime)
        server = try c.decodeIfPresent(ServerHealth.self, forKey: .server)
        // Tolerate null/missing arrays from older or future service versions.
        profiles = try c.decodeIfPresent([ProfileStatus].self, forKey: .profiles) ?? []
        findings = try c.decodeIfPresent([Finding].self, forKey: .findings) ?? []
        skippedProfiles = try c.decodeIfPresent([SkippedProfile].self, forKey: .skippedProfiles) ?? []
    }

    /// Limited view built from the unauthenticated `/api/summary`.
    public init(overall: String, summary: String) {
        self.overall = overall; self.summary = summary; uptime = nil; server = nil
        profiles = []; findings = []; skippedProfiles = []
    }

    /// Most recent finished backup across all profiles.
    public var lastBackup: Date? { profiles.compactMap { $0.last?.end }.max() }
}

enum GoTime {
    /// Go emits RFC 3339 with up to 9 fractional digits; ISO8601DateFormatter is only
    /// dependable with 3, so normalise first. Go's zero time (year 1) means "never".
    static func parse(_ s: String?) -> Date? {
        guard var str = s, !str.isEmpty else { return nil }
        if let dot = str.firstIndex(of: "."),
           let end = str[dot...].firstIndex(where: { $0 == "Z" || $0 == "+" || $0 == "-" }) {
            let frac = String(str[str.index(after: dot)..<end].prefix(3))
                .padding(toLength: 3, withPad: "0", startingAt: 0)
            str = String(str[..<dot]) + "." + frac + String(str[end...])
        }
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        var d = f.date(from: str)
        if d == nil {
            f.formatOptions = [.withInternetDateTime]
            d = f.date(from: str)
        }
        guard let date = d, date.timeIntervalSince1970 > 946_684_800 else { return nil } // after 2000
        return date
    }
}
