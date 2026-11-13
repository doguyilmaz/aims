import AppKit

@MainActor
final class AimsAppDelegate: NSObject, NSApplicationDelegate {
    let model = UsageModel()
    let panel = EdgePanelController()
    let dock = EdgeDockController()
    private var refreshTask: Task<Void, Never>?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        ProviderAssets.prepare()
        dock.start(model: model, openOverview: togglePanel)
        model.presentationDidChange = { [weak self] in
            self?.dock.update()
            self?.panel.reposition()
        }
        model.dockVisibilityDidChange = { [weak self] visible in
            self?.dock.setVisible(visible)
        }
        model.dockConfigurationDidChange = { [weak self] in
            self?.dock.configurationChanged()
        }
        model.dockPositionDidChange = { [weak self] in self?.dock.update() }
        refreshTask = Task { await model.run() }
        UserDefaults.standard.set(true, forKey: "hasLaunched")
        if model.fixtureURL != nil || UserDefaults.standard.bool(forKey: "edgePanelVisible") {
            togglePanel()
        }
    }

    func togglePanel() {
        dock.closePeek()
        if !model.panelVisible { dock.moveToPointerScreen() }
        panel.toggle(model: model, toggle: togglePanel)
    }

    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows flag: Bool) -> Bool {
        if !flag && !model.panelVisible { togglePanel() }
        return true
    }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        panel.prepareForTermination()
        dock.shutdown()
        refreshTask?.cancel()
        Task {
            await model.shutdown()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}
