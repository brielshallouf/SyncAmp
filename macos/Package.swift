// swift-tools-version:5.9
import PackageDescription

let package = Package(
    name: "SyncAmpMenu",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "SyncAmpMenu", targets: ["SyncAmpMenu"])],
    targets: [
        // Foundation-only: models + API client. No UI, so it is unit-testable anywhere.
        .target(name: "SyncAmpKit"),
        .executableTarget(name: "SyncAmpMenu", dependencies: ["SyncAmpKit"]),
        .testTarget(name: "SyncAmpKitTests", dependencies: ["SyncAmpKit"]),
    ]
)
