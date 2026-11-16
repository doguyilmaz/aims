import Foundation
import Observation
import UsageCore

@MainActor @Observable
final class UsageModel {
    let updater = AppUpdater()
    var snapshot: UsageSnapshot? { didSet { presentationDidChange?() } }
    var presentationDidChange: (@MainActor () -> Void)?
    var dockVisibilityDidChange: (@MainActor (Bool) -> Void)?
    var dockConfigurationDidChange: (@MainActor () -> Void)?
    var dockPositionDidChange: (@MainActor () -> Void)?
    var dockBehaviorDidChange: (@MainActor () -> Void)?
    var dockProvider: String?
    var dockPeekPinned = false
    var dockDisplays: [DockDisplay] = []
    var edgeDockDisplay = DockDisplaySelection(savedValue: UserDefaults.standard.string(forKey: "edgeDockDisplay")) {
        didSet {
            UserDefaults.standard.set(edgeDockDisplay.id, forKey: "edgeDockDisplay")
            dockConfigurationDidChange?()
        }
    }
    var edgeDockPinned = UserDefaults.standard.bool(forKey: "edgeDockPinned") {
        didSet {
            UserDefaults.standard.set(edgeDockPinned, forKey: "edgeDockPinned")
            dockBehaviorDidChange?()
        }
    }
    var edgeDockValueMode =
        DockValueMode(rawValue: UserDefaults.standard.string(forKey: "edgeDockValueMode") ?? "") ?? .used
    {
        didSet { UserDefaults.standard.set(edgeDockValueMode.rawValue, forKey: "edgeDockValueMode") }
    }
    private var edgeDockWindowFocus =
        UserDefaults.standard.dictionary(forKey: "edgeDockWindowFocus") as? [String: String] ?? [:]
    {
        didSet { UserDefaults.standard.set(edgeDockWindowFocus, forKey: "edgeDockWindowFocus") }
    }
    private var edgeDockAccountFocus =
        UserDefaults.standard.dictionary(forKey: "edgeDockAccountFocus") as? [String: String] ?? [:]
    {
        didSet { UserDefaults.standard.set(edgeDockAccountFocus, forKey: "edgeDockAccountFocus") }
    }
    var edgeDockPosition = EdgeDockLayout.normalizedPosition(
        UserDefaults.standard.object(forKey: "edgeDockPosition") as? Double ?? 0.5)
    {
        didSet {
            UserDefaults.standard.set(EdgeDockLayout.normalizedPosition(edgeDockPosition), forKey: "edgeDockPosition")
            dockPositionDidChange?()
        }
    }
    var edgeDockSide = EdgeDockSide(rawValue: UserDefaults.standard.string(forKey: "edgeDockSide") ?? "") ?? .right {
        didSet {
            UserDefaults.standard.set(edgeDockSide.rawValue, forKey: "edgeDockSide")
            dockConfigurationDidChange?()
        }
    }
    var edgeDockSize =
        EdgeDockSize(rawValue: UserDefaults.standard.string(forKey: "edgeDockSize") ?? "") ?? .standard
    {
        didSet {
            UserDefaults.standard.set(edgeDockSize.rawValue, forKey: "edgeDockSize")
            dockConfigurationDidChange?()
        }
    }
    var edgeDockAutoHide = UserDefaults.standard.object(forKey: "edgeDockAutoHide") as? Bool ?? true {
        didSet {
            UserDefaults.standard.set(edgeDockAutoHide, forKey: "edgeDockAutoHide")
            dockBehaviorDidChange?()
        }
    }
    var edgeDockMarkerStyle = EdgeDockMarkerStyle(
        savedValue: UserDefaults.standard.string(forKey: "edgeDockMarkerStyle"))
    {
        didSet {
            UserDefaults.standard.set(edgeDockMarkerStyle.rawValue, forKey: "edgeDockMarkerStyle")
            dockConfigurationDidChange?()
        }
    }
    var edgeDockEnabled = UserDefaults.standard.object(forKey: "edgeDockEnabled") as? Bool ?? true {
        didSet {
            UserDefaults.standard.set(edgeDockEnabled, forKey: "edgeDockEnabled")
            dockVisibilityDidChange?(edgeDockEnabled)
        }
    }
    var error: String?
    private(set) var summaryDate = Date()
    var isRefreshing = false
    var overviewPresentation: UsageOverviewPresentation = .hidden {
        didSet { if overviewPresentation != oldValue { dockBehaviorDidChange?() } }
    }
    var slimScrollbars = UserDefaults.standard.object(forKey: "slimScrollbars") as? Bool ?? false {
        didSet { UserDefaults.standard.set(slimScrollbars, forKey: "slimScrollbars") }
    }
    private(set) var configHome: String
    private(set) var additionalPaths = UserDefaults.standard.string(forKey: "additionalPaths") ?? ""
    private var command: UsageCommand?
    private var shuttingDown = false
    private var nextProviderRefresh = ContinuousClock.now
    private var providerRefreshError: String?
    private let commandRunner: (@MainActor ([String]) async throws -> Data)?
    var displayStyle =
        UsageDisplayStyle(rawValue: UserDefaults.standard.string(forKey: "displayStyle") ?? "") ?? .compact
    {
        didSet {
            UserDefaults.standard.set(displayStyle.rawValue, forKey: "displayStyle")
            presentationDidChange?()
        }
    }

    init(configHomeOverride: String? = nil, commandRunner: (@MainActor ([String]) async throws -> Data)? = nil) {
        self.commandRunner = commandRunner
        if let configHomeOverride {
            configHome = configHomeOverride
            return
        }
        let stored = UserDefaults.standard.string(forKey: "configHome")
        configHome = stored ?? ProcessInfo.processInfo.environment["AIMS_HOME"] ?? ""
        if stored == nil && !configHome.isEmpty { UserDefaults.standard.set(configHome, forKey: "configHome") }
    }

    func dockFocus(for providerID: String) -> DockWindowFocus {
        DockWindowFocus(savedValue: edgeDockWindowFocus[providerID])
    }

    func dockAccount(for providerID: String) -> DockAccountFocus {
        guard DockAccountFocus.supports(providerID) else { return .all }
        return DockAccountFocus(savedValue: edgeDockAccountFocus[providerID])
    }

    func setDockAccount(_ account: DockAccountFocus, for providerID: String) {
        guard DockAccountFocus.supports(providerID) else { return }
        if account == .all {
            edgeDockAccountFocus.removeValue(forKey: providerID)
        } else if case .account(let id) = account, !id.isEmpty {
            edgeDockAccountFocus[providerID] = account.id
        }
    }

    func setDockFocus(_ focus: DockWindowFocus, for providerID: String) {
        if focus == .automatic {
            edgeDockWindowFocus.removeValue(forKey: providerID)
        } else {
            edgeDockWindowFocus[providerID] = focus.id
        }
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
        if automaticRefresh && fixtureURL == nil { await refresh() }
        while !Task.isCancelled {
            do { try await Task.sleep(for: .seconds(60)) } catch { return }
            summaryDate = Date()
            if automaticRefresh && fixtureURL == nil && ContinuousClock.now >= nextProviderRefresh {
                await refresh()
            } else {
                await reloadCached()
            }
        }
    }

    // Other aims sessions/status hooks may update caches between API polls.
    // A local read must not clear a failed real request's error indication.
    func reloadCached() async { await readSnapshot(live: false, preserveRefreshError: true) }

    func refresh(live: Bool = true) async { await readSnapshot(live: live, preserveRefreshError: false) }

    private func readSnapshot(live: Bool, preserveRefreshError: Bool) async {
        guard !isRefreshing, !shuttingDown else { return }
        isRefreshing = true
        defer {
            isRefreshing = false
            command = nil
            if live { nextProviderRefresh = ContinuousClock.now.advanced(by: .seconds(300)) }
        }
        do {
            let data: Data
            if commandRunner == nil, let fixtureURL {
                data = try Data(contentsOf: fixtureURL)
            } else {
                data = try await runCommand(["usage", "--json"] + (live ? ["--refresh"] : []))
            }
            try Task.checkCancellation()
            snapshot = try UsageSnapshot.decode(data)
            summaryDate = Date()
            if !preserveRefreshError { providerRefreshError = nil }
            error = providerRefreshError
        } catch is CancellationError {
        } catch {
            if live { providerRefreshError = error.localizedDescription }
            self.error = error.localizedDescription
        }
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
            let data = try await runCommand(arguments)
            try Task.checkCancellation()
            snapshot = try UsageSnapshot.decode(data)
            error = providerRefreshError
            return true
        } catch is CancellationError {
            return false
        } catch {
            self.error = error.localizedDescription
            return false
        }
    }

    private func runCommand(_ arguments: [String]) async throws -> Data {
        if let commandRunner { return try await commandRunner(arguments) }
        let command = UsageCommand()
        self.command = command
        return try await command.run(
            executable: executable(), arguments: arguments, configHome: configHome, additionalPaths: additionalPaths)
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
