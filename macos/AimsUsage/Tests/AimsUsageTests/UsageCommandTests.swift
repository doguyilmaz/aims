import Foundation
import Testing

@testable import AimsUsage

@MainActor @Test func capturesOutputWithoutPipeDeadlock() async throws {
    let command = UsageCommand()
    let data = try await command.run(
        executable: URL(fileURLWithPath: "/usr/bin/printf"), arguments: ["%s", "fixture output"])
    #expect(String(decoding: data, as: UTF8.self) == "fixture output")
}

@MainActor @Test func cancellationStopsTheOwnedProcess() async throws {
    let command = UsageCommand()
    let task = Task { try await command.run(executable: URL(fileURLWithPath: "/bin/sleep"), arguments: ["30"]) }
    // Yield lets the main actor start the command; cancellation is valid before
    // or after launch, and must always complete without waiting for the sleep.
    await Task.yield()
    task.cancel()
    await #expect(throws: CancellationError.self) { try await task.value }
}

@MainActor @Test func selectedDataFolderReachesTheOwnedCLI() async throws {
    let data = try await UsageCommand().run(
        executable: URL(fileURLWithPath: "/usr/bin/printenv"), arguments: ["AIMS_HOME"],
        configHome: "/fixture/work-aims")
    #expect(
        String(decoding: data, as: UTF8.self).trimmingCharacters(in: .whitespacesAndNewlines) == "/fixture/work-aims")
}
