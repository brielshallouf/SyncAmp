import XCTest
@testable import SyncAmpKit

final class ModelsTests: XCTestCase {
    // Captured from the Go service's /api/status shape.
    let json = """
    {"overall":"error","summary":"1 profile(s) failed on last run","os":"darwin/arm64","uptime":"12s",
     "server":{"url":"https://backup.lan:5800","up":true},
     "profiles":[
       {"name":"Documents","path":"/x","running":false,"scheduled":true,"paused":false,
        "last":{"profile":"Documents","start":"2026-11-27T18:56:09.123456789Z","end":"2026-11-27T18:56:10.5-08:00","status":"success","exitCode":0,"trigger":"manual"}},
       {"name":"Photos","path":"/y","running":false,"scheduled":true,"paused":false,
        "last":{"profile":"Photos","start":"2026-11-27T18:56:07Z","end":"2026-11-27T18:56:08Z","status":"failed","error":"unable to connect","trigger":"schedule"}},
       {"name":"Scratch","path":"/z","running":false,"scheduled":false,"paused":true}
     ],
     "findings":[{"level":"WARN","msg":"x"}]}
    """

    func testDecodesServiceStatus() throws {
        let s = try JSONDecoder().decode(Status.self, from: Data(json.utf8))
        XCTAssertEqual(s.overall, "error")
        XCTAssertEqual(s.profiles.count, 3)
        XCTAssertEqual(s.profiles[1].last?.error, "unable to connect")
        XCTAssertTrue(s.profiles[2].paused)
        XCTAssertNil(s.profiles[2].last)
        XCTAssertEqual(s.findings.first?.level, "WARN")
        XCTAssertNotNil(s.lastBackup)
    }

    func testNullAndMissingArraysDecodeAsEmpty() throws {
        let s = try JSONDecoder().decode(Status.self, from: Data(#"{"overall":"warn","summary":"none","profiles":null}"#.utf8))
        XCTAssertTrue(s.profiles.isEmpty); XCTAssertTrue(s.findings.isEmpty)
    }

    func testGoTimeVariants() {
        XCTAssertNotNil(GoTime.parse("2026-11-27T18:56:10.123456789Z"))
        XCTAssertNotNil(GoTime.parse("2026-11-27T18:56:10.5-08:00"))
        XCTAssertNotNil(GoTime.parse("2026-11-27T18:56:10Z"))
        XCTAssertNil(GoTime.parse("0001-01-01T00:00:00Z"), "Go zero time means never")
        XCTAssertNil(GoTime.parse(nil)); XCTAssertNil(GoTime.parse("garbage"))
    }
}
