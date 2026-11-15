// swift-tools-version: 6.2.0
import PackageDescription

let package = Package(
    name: "AimsUsage",
    platforms: [.macOS(.v14)],
    products: [.executable(name: "AimsUsage", targets: ["AimsUsage"])],
    dependencies: [.package(url: "https://github.com/sparkle-project/Sparkle", exact: "2.10.1")],
    targets: [
        .target(name: "UsageCore"),
        .executableTarget(
            name: "AimsUsage", dependencies: ["UsageCore", .product(name: "Sparkle", package: "Sparkle")],
            swiftSettings: [.defaultIsolation(MainActor.self)],
            linkerSettings: [.unsafeFlags(["-Xlinker", "-rpath", "-Xlinker", "@executable_path/../Frameworks"])]
        ),
        .testTarget(name: "UsageCoreTests", dependencies: ["UsageCore"]),
        .testTarget(name: "AimsUsageTests", dependencies: ["AimsUsage", "UsageCore"]),
    ],
    swiftLanguageModes: [.v6]
)
