import AppKit
import SwiftUI

@MainActor
final class EdgePanelController: NSObject, NSWindowDelegate {
    private var panel: NSPanel?
    private weak var model: UsageModel?
    private var screen: NSScreen?
    private var isTerminating = false

    func prepareForTermination() { isTerminating = true }

    func toggle(model: UsageModel, toggle: @escaping @MainActor () -> Void) {
        if let panel, panel.isVisible {
            panel.close()
            return
        }
        self.model = model
        if panel == nil {
            let panel = UsagePanel(
                contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel],
                backing: .buffered, defer: false)
            panel.title = "Aims Usage"
            panel.isOpaque = false
            panel.backgroundColor = .clear
            panel.hasShadow = true
            panel.isFloatingPanel = true
            panel.level = .floating
            panel.hidesOnDeactivate = false
            panel.isReleasedWhenClosed = false
            panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
            panel.delegate = self
            panel.contentView = NSHostingView(
                rootView: UsageDashboard(model: model, togglePanel: toggle, surface: .panel)
                    .clipShape(RoundedRectangle(cornerRadius: 14)).ignoresSafeArea())
            self.panel = panel
            NotificationCenter.default.addObserver(
                self, selector: #selector(screenChanged), name: NSApplication.didChangeScreenParametersNotification,
                object: nil)
        }
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        position()
        panel?.orderFrontRegardless()
        model.panelVisible = true
        UserDefaults.standard.set(true, forKey: "edgePanelVisible")
    }

    @objc private func screenChanged() {
        if let old = screen, !NSScreen.screens.contains(old) { screen = NSScreen.main }
        position()
    }

    private func position() {
        guard let panel, let frame = (screen ?? NSScreen.main)?.visibleFrame else { return }
        let width = min(400, frame.width - 24)
        let height = min(780, frame.height - 24)
        // visibleFrame excludes the menu bar and the Dock on any edge.
        panel.setFrame(
            NSRect(x: frame.maxX - width - 12, y: frame.maxY - height - 12, width: width, height: height), display: true
        )
    }

    func windowWillClose(_ notification: Notification) {
        // AppKit also closes the panel when the app quits. Keep the user's
        // open-panel preference so the next launch can restore it.
        guard !isTerminating else { return }
        model?.panelVisible = false
        UserDefaults.standard.set(false, forKey: "edgePanelVisible")
    }
}
