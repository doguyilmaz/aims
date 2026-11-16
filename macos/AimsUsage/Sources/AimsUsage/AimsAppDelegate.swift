import AppKit

@MainActor
final class AimsAppDelegate: NSObject, NSApplicationDelegate {
    let model = UsageModel()
    let overview = UsageOverviewController()
    let status = UsageStatusItemController()
    let dock = EdgeDockController()
    private var refreshTask: Task<Void, Never>?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        ProviderAssets.prepare()
        model.updater.start()
        overview.start(model: model) { [weak self] source in
            switch source {
            case .menuBar: self?.status.anchor
            case .dock: self?.dock.overviewAnchor
            }
        }
        status.start(model: model) { [weak self] in self?.toggleOverview(from: .menuBar) }
        dock.start(
            model: model, openOverview: { [weak self] in self?.showOverview(from: .dock) },
            closeOverview: { [weak self] in self?.overview.close() })
        model.presentationDidChange = { [weak self] in
            self?.dock.update()
            self?.overview.reposition()
        }
        model.dockVisibilityDidChange = { [weak self] visible in
            self?.dock.setVisible(visible)
        }
        model.dockConfigurationDidChange = { [weak self] in
            self?.dock.configurationChanged()
            self?.overview.reposition()
        }
        model.dockPositionDidChange = { [weak self] in
            self?.dock.update()
            self?.overview.reposition()
        }
        model.dockBehaviorDidChange = { [weak self] in self?.dock.update() }
        refreshTask = Task { await model.run() }
        UserDefaults.standard.set(true, forKey: "hasLaunched")
        if UserDefaults.standard.bool(forKey: "edgePanelVisible") {
            overview.restorePinned()
        } else if model.fixtureURL != nil {
            showOverview(from: .dock)
        }
    }

    func toggleOverview(from source: UsageOverviewSource) {
        dock.closePeek()
        overview.toggle(from: source)
    }

    func showOverview(from source: UsageOverviewSource) {
        dock.closePeek()
        overview.show(from: source)
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag && !model.overviewPresentation.isVisible { showOverview(from: .menuBar) }
        return true
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        overview.prepareForTermination()
        status.stop()
        dock.shutdown()
        refreshTask?.cancel()
        Task {
            await model.shutdown()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}
