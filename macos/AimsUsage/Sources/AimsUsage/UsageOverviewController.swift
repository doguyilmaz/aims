import AppKit
import SwiftUI

@MainActor
final class UsageOverviewController: NSObject, NSPopoverDelegate {
    private let popover = NSPopover()
    private weak var model: UsageModel?
    private var resolveAnchor: (@MainActor (UsageOverviewSource) -> UsageOverviewAnchor?)?
    private var host: UsageOverviewHostingController?
    private var desiredHeight: CGFloat = 420
    private var configuration = 0
    private var nativeClosing = false
    private var isTerminating = false

    func start(model: UsageModel, anchors: @escaping @MainActor (UsageOverviewSource) -> UsageOverviewAnchor?) {
        self.model = model
        resolveAnchor = anchors
        popover.delegate = self
        NotificationCenter.default.addObserver(
            self, selector: #selector(screenChanged), name: NSApplication.didChangeScreenParametersNotification,
            object: nil)
    }

    func toggle(from source: UsageOverviewSource) {
        guard let model, !focusSheetIfPresent() else { return }
        let source = source == .dock && !model.edgeDockEnabled ? .menuBar : source
        model.overviewPresentation.toggle(
            from: source, pinnedWhenOpening: UserDefaults.standard.bool(forKey: "edgePanelVisible"))
        if case .closing(_, nil) = model.overviewPresentation {
            UserDefaults.standard.set(false, forKey: "edgePanelVisible")
        }
        drive()
    }

    func restorePinned() {
        guard let model, !model.overviewPresentation.isVisible else { return }
        let source: UsageOverviewSource = model.edgeDockEnabled ? .dock : .menuBar
        model.overviewPresentation = .presented(UsageOverviewRequest(source: source, pinned: true))
        drive()
    }

    func show(from source: UsageOverviewSource) {
        guard let model, !focusSheetIfPresent() else { return }
        let source = source == .dock && !model.edgeDockEnabled ? .menuBar : source
        model.overviewPresentation.show(
            from: source, pinnedWhenOpening: UserDefaults.standard.bool(forKey: "edgePanelVisible"))
        drive()
        if popover.isShown { host?.view.window?.makeKeyAndOrderFront(nil) }
    }

    func togglePin() {
        guard let model, case .presented = model.overviewPresentation, !focusSheetIfPresent() else { return }
        model.overviewPresentation.togglePin()
        popover.behavior = model.overviewPresentation.isPinned ? .applicationDefined : .transient
        UserDefaults.standard.set(model.overviewPresentation.isPinned, forKey: "edgePanelVisible")
    }

    func close() {
        guard let model, !focusSheetIfPresent() else { return }
        UserDefaults.standard.set(false, forKey: "edgePanelVisible")
        model.overviewPresentation.close()
        drive()
    }

    func prepareForTermination() {
        isTerminating = true
        NotificationCenter.default.removeObserver(self)
        popover.delegate = nil
        popover.close()
    }

    func reposition() {
        guard let model, case .presented(let request) = model.overviewPresentation,
            let anchor = resolveAnchor?(request.source)
        else { return }
        guard configureContent(anchor: anchor) else { return }
        if popover.isShown {
            popover.positioningRect = anchor.rect
        } else if !nativeClosing {
            drive()
        }
    }

    private func drive() {
        guard !isTerminating, let model else { return }
        switch model.overviewPresentation {
        case .hidden: return
        case .closing:
            if popover.isShown && !nativeClosing { popover.performClose(nil) }
        case .presented(let request):
            guard !popover.isShown, !nativeClosing else { return }
            guard let anchor = resolveAnchor?(request.source), anchor.view.window != nil, !anchor.rect.isEmpty else {
                model.overviewPresentation = .hidden
                return
            }
            guard configureContent(anchor: anchor) else {
                model.overviewPresentation = .hidden
                return
            }
            popover.behavior = request.pinned ? .applicationDefined : .transient
            popover.animates = !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
            popover.show(relativeTo: anchor.rect, of: anchor.view, preferredEdge: anchor.edge)
            host?.view.window?.title = "Aims Usage"
            host?.view.window?.makeKey()
        }
    }

    private func configureContent(anchor: UsageOverviewAnchor) -> Bool {
        guard let model, let frame = anchor.view.window?.screen?.visibleFrame, !frame.isEmpty else { return false }
        configuration += 1
        let version = configuration
        let width = UsageOverviewLayout.width(in: frame, style: model.displayStyle)
        let maximum = UsageOverviewLayout.maximumBodyHeight(in: frame, style: model.displayStyle)
        guard width > 0 else { return false }
        let root = AnyView(
            UsageDashboard(
                model: model, togglePin: { [weak self] in self?.togglePin() },
                close: { [weak self] in self?.close() }, width: width, maximumBodyHeight: maximum,
                heightDidChange: { [weak self] height in
                    guard let self, self.configuration == version else { return }
                    self.desiredHeight = height
                    if !self.nativeClosing {
                        self.resize(width: width, height: min(height, maximum + model.displayStyle.chromeHeight))
                    }
                }))
        if let host {
            host.rootView = root
        } else {
            let host = UsageOverviewHostingController(rootView: root)
            host.onCancel = { [weak self] in self?.close() }
            self.host = host
            popover.contentViewController = host
        }
        resize(width: width, height: min(desiredHeight, maximum + model.displayStyle.chromeHeight))
        return true
    }

    private func resize(width: CGFloat, height: CGFloat) {
        let size = NSSize(width: width, height: max(0, height))
        if popover.contentSize != size {
            host?.preferredContentSize = size
            popover.contentSize = size
        }
    }

    private func focusSheetIfPresent() -> Bool {
        guard let sheet = host?.view.window?.attachedSheet else { return false }
        sheet.makeKeyAndOrderFront(nil)
        return true
    }

    func popoverShouldClose(_ popover: NSPopover) -> Bool { host?.view.window?.attachedSheet == nil }

    func popoverWillClose(_ notification: Notification) {
        guard !isTerminating else { return }
        nativeClosing = true
        model?.overviewPresentation.willClose()
    }

    func popoverDidClose(_ notification: Notification) {
        guard !isTerminating else { return }
        nativeClosing = false
        model?.overviewPresentation.didClose()
        drive()
    }

    @objc private func screenChanged() {
        if model?.overviewPresentation.isVisible == false && UserDefaults.standard.bool(forKey: "edgePanelVisible") {
            restorePinned()
        } else {
            reposition()
        }
    }
}
