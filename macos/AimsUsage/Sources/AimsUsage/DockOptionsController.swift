import AppKit
import SwiftUI

@MainActor
final class DockOptionsController: NSObject, NSWindowDelegate {
    private var panel: UsagePanel?
    private(set) var providerID: String?
    var visibilityDidChange: (@MainActor () -> Void)?
    var isPresented: Bool { panel != nil }

    func show(
        model: UsageModel, providerID: String?, dock: CGRect, side: EdgeDockSide, screen: CGRect,
        pointerY: CGFloat, openOverview: @escaping @MainActor () -> Void
    ) {
        close()
        self.providerID = providerID
        let panel = UsagePanel(
            contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        panel.title = "Dock options"
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.isFloatingPanel = true
        panel.level = NSWindow.Level(rawValue: NSWindow.Level.floating.rawValue + 1)
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        panel.delegate = self
        self.panel = panel
        panel.onCancel = { [weak self, weak panel] in
            guard self?.panel === panel else { return }
            self?.close()
        }
        let maximumBodyHeight = max(0, screen.height - 24 - 43)
        panel.contentView = UsageRoundedHostingView(
            rootView: AnyView(
                DockOptionsView(
                    model: model, providerID: providerID, width: DockOptionsLayout.width(in: screen),
                    maximumBodyHeight: maximumBodyHeight, close: { [weak self] in self?.close() },
                    openOverview: openOverview,
                    heightDidChange: { [weak self, weak panel] height in
                        guard self?.panel === panel else { return }
                        panel?.setFrame(
                            DockOptionsLayout.frame(
                                in: screen, dock: dock, side: side, pointerY: pointerY, height: height),
                            display: true)
                    })), cornerRadius: 12)
        panel.setFrame(
            DockOptionsLayout.frame(in: screen, dock: dock, side: side, pointerY: pointerY, height: 343), display: true)
        visibilityDidChange?()
        panel.presentForInteraction()
    }

    func close() {
        guard let panel else { return }
        self.panel = nil
        providerID = nil
        panel.onCancel = nil
        panel.delegate = nil
        panel.orderOut(nil)
        visibilityDidChange?()
    }

    func windowDidResignKey(_ notification: Notification) {
        if notification.object as? NSWindow === panel { close() }
    }
}
