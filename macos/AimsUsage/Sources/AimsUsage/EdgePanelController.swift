import AppKit
import SwiftUI

@MainActor
final class EdgePanelController: NSObject, NSWindowDelegate {
    private var panel: UsagePanel?
    private weak var model: UsageModel?
    private var screen: NSScreen?
    private var isTerminating = false
    private var desiredHeight: CGFloat = 420
    private var toggleAction: (@MainActor () -> Void)?
    private var hostingView: NSHostingView<AnyView>?

    func prepareForTermination() { isTerminating = true }

    func toggle(model: UsageModel, toggle: @escaping @MainActor () -> Void) {
        if let panel, panel.isVisible {
            panel.close()
            return
        }
        self.model = model
        toggleAction = toggle
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        if panel == nil {
            let panel = UsagePanel(
                contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
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
            self.panel = panel
            NotificationCenter.default.addObserver(
                self, selector: #selector(screenChanged), name: NSApplication.didChangeScreenParametersNotification,
                object: nil)
        }
        configureContent()
        panel?.makeKeyAndOrderFront(nil)
        model.panelVisible = true
        UserDefaults.standard.set(true, forKey: "edgePanelVisible")
    }

    func reposition() { if panel != nil { configureContent() } }

    @objc private func screenChanged() {
        if let old = screen, !NSScreen.screens.contains(old) { screen = NSScreen.main }
        configureContent()
    }

    private func configureContent() {
        guard let panel, let model, let toggleAction, let frame = (screen ?? NSScreen.main)?.visibleFrame else {
            return
        }
        let reserved: CGFloat = model.edgeDockEnabled ? EdgeDockLayout.width + EdgeDockLayout.gap : 12
        let width = min(model.displayStyle.preferredWidth, frame.width - reserved - 24)
        let maximumBody = max(160, frame.height - 24 - 84)
        position()
        let root = AnyView(
            UsageDashboard(
                model: model, togglePanel: toggleAction, surface: .panel, width: width, maximumBodyHeight: maximumBody,
                heightDidChange: { [weak self] height in
                    self?.desiredHeight = height
                    self?.position()
                }
            )
            .frame(maxHeight: .infinity, alignment: .top)
            .clipShape(RoundedRectangle(cornerRadius: 18)).ignoresSafeArea())
        if let hostingView {
            hostingView.rootView = root
        } else {
            let host = NSHostingView(rootView: root)
            hostingView = host
            panel.contentView = host
        }
    }

    private func position() {
        guard let panel, let frame = (screen ?? NSScreen.main)?.visibleFrame else { return }
        let reserved: CGFloat = model?.edgeDockEnabled == true ? EdgeDockLayout.width + EdgeDockLayout.gap : 12
        let width = min(model?.displayStyle.preferredWidth ?? 400, frame.width - reserved - 24)
        let height = min(desiredHeight, frame.height - 24)
        // visibleFrame excludes a Dock at the bottom or either edge.
        let nextFrame = NSRect(
            x: frame.maxX - reserved - width, y: frame.maxY - height - 12, width: width, height: height)
        let keyboard = NSApp.currentEvent?.type == .keyDown || NSApp.currentEvent?.type == .keyUp
        let resizeOnly = abs(panel.frame.minX - nextFrame.minX) < 1 && abs(panel.frame.maxY - nextFrame.maxY) < 1
        if panel.isVisible && resizeOnly && !keyboard && !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion {
            NSAnimationContext.runAnimationGroup { context in
                context.duration = 0.18
                panel.animator().setFrame(nextFrame, display: true)
            }
        } else {
            panel.setFrame(nextFrame, display: true)
        }
    }

    func windowWillClose(_ notification: Notification) {
        guard !isTerminating else { return }
        model?.panelVisible = false
        UserDefaults.standard.set(false, forKey: "edgePanelVisible")
    }
}
