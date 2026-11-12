import AppKit

@MainActor
final class AimsAppDelegate: NSObject, NSApplicationDelegate {
    let model = UsageModel()
    let panel = EdgePanelController()
    private var refreshTask: Task<Void, Never>?

    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.accessory)
        refreshTask = Task { await model.run() }
        let firstLaunch = UserDefaults.standard.object(forKey: "hasLaunched") == nil
        UserDefaults.standard.set(true, forKey: "hasLaunched")
        if firstLaunch || model.fixtureURL != nil || UserDefaults.standard.bool(forKey: "edgePanelVisible") {
            togglePanel()
        }
    }

    func togglePanel() { panel.toggle(model: model, toggle: togglePanel) }

    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        panel.prepareForTermination()
        refreshTask?.cancel()
        Task {
            await model.shutdown()
            sender.reply(toApplicationShouldTerminate: true)
        }
        return .terminateLater
    }
}
