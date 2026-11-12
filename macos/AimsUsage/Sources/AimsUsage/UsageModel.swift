import Foundation
import Observation
import UsageCore

@MainActor @Observable
final class UsageModel {
    var snapshot: UsageSnapshot? { didSet { presentationDidChange?() } }
    var presentationDidChange: (@MainActor () -> Void)?
    var dockVisibilityDidChange: (@MainActor (Bool) -> Void)?
    var dockProvider: String?
    var dockPeekPinned = false
    var edgeDockEnabled = UserDefaults.standard.object(forKey: "edgeDockEnabled") as? Bool ?? true {
        didSet {
            UserDefaults.standard.set(edgeDockEnabled, forKey: "edgeDockEnabled")
            dockVisibilityDidChange?(edgeDockEnabled)
        }
    }
    var error: String?
    private(set) var summaryDate = Date()
    var isRefreshing = false
    var panelVisible = false
    private(set) var configHome: String
    private(set) var additionalPaths = UserDefaults.standard.string(forKey: "additionalPaths") ?? ""
    private var command: UsageCommand?
    private var shuttingDown = false
    var displayStyle =
        UsageDisplayStyle(rawValue: UserDefaults.standard.string(forKey: "displayStyle") ?? "") ?? .compact
    {
        didSet { UserDefaults.standard.set(displayStyle.rawValue, forKey: "displayStyle") }
    }

    init() {
        let stored = UserDefaults.standard.string(forKey: "configHome")
        configHome = stored ?? ProcessInfo.processInfo.environment["AIMS_HOME"] ?? ""
        if stored == nil && !configHome.isEmpty { UserDefaults.standard.set(configHome, forKey: "configHome") }
    }

    func applyPaths(configHome: String, additionalPaths: String) async -> Bool {
        guard !isRefreshing, !shuttingDown else { return false }
        do {
            let home = FileManager.default.homeDirectoryForCurrentUser.path
            _ = try UsageEnvironment.make(
                base: [:], homeDirectory: home, aimsHome: configHome, additionalPaths: additionalPaths)
            self.configHome = try UsageEnvironment.homePath(configHome, homeDirectory: home)
            self.additionalPaths = additionalPaths
            UserDefaults.standard.set(self.configHome, forKey: "configHome")
            UserDefaults.standard.set(additionalPaths, forKey: "additionalPaths")
            snapshot = nil
            error = nil
            await refresh(live: false)
            return error == nil
        } catch {
            self.error = error.localizedDescription
            return false
        }
    }

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
        return MenuSummary.value(snapshot: snapshot, now: summaryDate, globalError: error)
    }

    func run() async {
        await refresh(live: false)
        var nextRefresh = Date()
        while !Task.isCancelled {
            summaryDate = Date()
            if automaticRefresh && fixtureURL == nil && summaryDate >= nextRefresh {
                await refresh(live: true)
                nextRefresh = Date().addingTimeInterval(300)
            }
            do { try await Task.sleep(for: .seconds(60)) } catch { return }
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
                    executable: executable(), arguments: ["usage", "--json"] + (live ? ["--refresh"] : []),
                    configHome: configHome, additionalPaths: additionalPaths)
            }
            try Task.checkCancellation()
            snapshot = try UsageSnapshot.decode(data)
            summaryDate = Date()
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
            let data = try await command.run(
                executable: executable(), arguments: arguments, configHome: configHome, additionalPaths: additionalPaths
            )
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
