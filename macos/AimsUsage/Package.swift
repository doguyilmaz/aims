// swift-tools-version: 6.2.0
import PackageDescription

let package = Package(
    name: "AimsUsage",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "AimsUsage", targets: ["AimsUsage"])],
    targets: [
        .target(name: "UsageCore"),
        .executableTarget(
            name: "AimsUsage", dependencies: ["UsageCore"],
            swiftSettings: [.defaultIsolation(MainActor.self)]
        ),
        .testTarget(name: "UsageCoreTests", dependencies: ["UsageCore"]),
        .testTarget(name: "AimsUsageTests", dependencies: ["AimsUsage", "UsageCore"]),
    ],
    swiftLanguageModes: [.v6]
)
