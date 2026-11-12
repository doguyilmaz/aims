import Foundation
import Observation
import UsageCore

@MainActor @Observable
final class UsageModel {
    var snapshot: UsageSnapshot?
    var error: String?
    var isRefreshing = false
    var panelVisible = false
    private var command: UsageCommand?
    private var shuttingDown = false

    func shutdown() async {
        shuttingDown = true
        await command?.cancelAndWait()
    }
    var automaticRefresh = UserDefaults.standard.object(forKey: "automaticRefresh") as? Bool ?? true {
        didSet { UserDefaults.standard.set(automaticRefresh, forKey: "automaticRefresh") }
    }
    var showMenuPercent = UserDefaults.standard.object(forKey: "showMenuPercent") as? Bool ?? true {
        didSet { UserDefaults.standard.set(showMenuPercent, forKey: "showMenuPercent") }
    }
    var executablePath = UserDefaults.standard.string(forKey: "executablePath") ?? "" {
        didSet { UserDefaults.standard.set(executablePath, forKey: "executablePath") }
    }

    var fixtureURL: URL? {
        if let path = ProcessInfo.processInfo.environment["AIMS_USAGE_FIXTURE"] { return URL(fileURLWithPath: path) }
        if Bundle.main.object(forInfoDictionaryKey: "AimsUsagePreview") as? Bool == true {
            return Bundle.main.url(forResource: "usage-preview", withExtension: "json")
        }
        return nil
    }

    var menuSummary: String? {
        guard showMenuPercent else { return nil }
        let now = Date()
        let values =
            snapshot?.providers.filter { $0.installed && ($0.error == nil || $0.error == "") }.flatMap(\.rows).filter {
                !$0.isStale(at: now) && $0.error == nil
            }
            .flatMap { $0.windows ?? [] }.filter { !$0.needsRefresh(at: now) }.compactMap(\.validPercent) ?? []
        return values.max().map { $0 >= 1_000 ? "999%+" : String(format: "%.0f%%", $0) }
    }

    func run() async {
        await refresh(live: false)
        while !Task.isCancelled {
            if automaticRefresh && fixtureURL == nil { await refresh(live: true) }
            do { try await Task.sleep(for: .seconds(300)) } catch { return }
        }
    }

    func refresh(live: Bool = true) async {
        guard !isRefreshing, !shuttingDown else { return }
        isRefreshing = true
        defer {
            isRefreshing = false
            command = nil
        }
        do {
            let data: Data
            if let fixtureURL {
                data = try Data(contentsOf: fixtureURL)
            } else {
                let command = UsageCommand()
                self.command = command
                data = try await command.run(
                    executable: executable(), arguments: ["usage", "--json"] + (live ? ["--refresh"] : []))
            }
            try Task.checkCancellation()
            snapshot = try UsageSnapshot.decode(data)
            error = nil
        } catch is CancellationError {} catch { self.error = error.localizedDescription }
    }

    func addReminder(provider: String, row: String, label: String, at: Date) async -> Bool {
        await changeReminder([
            "usage", "reset", "add", provider, row, "--label", label, "--at", at.ISO8601Format(), "--json",
        ])
    }

    func removeReminder(id: String) async {
        _ = await changeReminder(["usage", "reset", "remove", id, "--json"])
    }

    private func changeReminder(_ arguments: [String]) async -> Bool {
        guard !isRefreshing, !shuttingDown, fixtureURL == nil else { return false }
        isRefreshing = true
        defer {
            isRefreshing = false
            command = nil
        }
        do {
            let command = UsageCommand()
            self.command = command
            let data = try await command.run(executable: executable(), arguments: arguments)
            snapshot = try UsageSnapshot.decode(data)
            error = nil
            return true
        } catch {
            self.error = error.localizedDescription
            return false
        }
    }

    private func executable() throws -> URL {
        if !executablePath.isEmpty {
            guard executablePath.hasPrefix("/"), FileManager.default.isExecutableFile(atPath: executablePath) else {
                throw UsageError.missingCLI
            }
            return URL(fileURLWithPath: executablePath)
        }
        if let bundled = Bundle.main.url(forResource: "aims", withExtension: nil),
            FileManager.default.isExecutableFile(atPath: bundled.path)
        {
            return bundled
        }
        for path in ["/opt/homebrew/bin/aims", "/usr/local/bin/aims"]
        where FileManager.default.isExecutableFile(atPath: path) { return URL(fileURLWithPath: path) }
        throw UsageError.missingCLI
    }
}
