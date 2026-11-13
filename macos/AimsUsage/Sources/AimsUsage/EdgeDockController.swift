import AppKit
import SwiftUI

@MainActor
final class EdgeDockController: NSObject {
    private var dock: UsagePanel?
    private var handle: UsagePanel?
    private var peek: UsagePanel?
    private var peekHostingView: NSHostingView<AnyView>?
    private weak var model: UsageModel?
    private var screen: NSScreen?
    private var peekHeight: CGFloat = 240
    private let pointerMonitor = EdgeDockPointerMonitor()
    private var openOverview: (@MainActor () -> Void)?
    private var expanded = false
    private var hasPresented = false
    private var presentationGeneration = 0
    private var shuttingDown = false

    func start(model: UsageModel, openOverview: @escaping @MainActor () -> Void) {
        self.model = model
        self.openOverview = openOverview
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        dock = makePanel(title: "Aims Edge Dock")
        handle = makePanel(title: "Aims Dock edge marker")
        handle?.hasShadow = false
        configureContent()
        NotificationCenter.default.addObserver(
            self, selector: #selector(screenChanged),
            name: NSApplication.didChangeScreenParametersNotification, object: nil)
        setVisible(model.edgeDockEnabled)
    }

    private func configureContent() {
        guard let dock, let model, let openOverview else { return }
        dock.contentView = NSHostingView(
            rootView: EdgeDockView(
                model: model, maximumHeight: (screen?.visibleFrame.height ?? 800) - 16,
                hoverProvider: { [weak self] id in self?.present(id, pin: false) },
                selectProvider: { [weak self] id in self?.present(id, pin: true) },
                openOverview: { [weak self] in
                    self?.closePeek()
                    openOverview()
                }))
        handle?.contentView = NSHostingView(
            rootView: EdgeDockHandleView(side: model.edgeDockSide, reveal: { [weak self] in self?.reveal() }))
    }

    func configurationChanged() {
        configureContent()
        configurePeekContent()
        update()
    }

    func setVisible(_ visible: Bool) {
        if visible && !shuttingDown {
            update()
        } else {
            presentationGeneration += 1
            hasPresented = false
            expanded = false
            dismissPeek()
            pointerMonitor.stop()
            dock?.orderOut(nil)
            handle?.orderOut(nil)
        }
    }

    func update() {
        guard !shuttingDown, let model, let dock, let frame = (screen ?? NSScreen.main)?.visibleFrame else { return }
        guard model.edgeDockEnabled else {
            setVisible(false)
            return
        }
        let dockFrame = EdgeDockLayout.frame(
            in: frame, providerCount: model.snapshot?.providers.count ?? 0,
            side: model.edgeDockSide, size: model.edgeDockSize, position: model.edgeDockPosition)
        dock.setFrame(dockFrame, display: true)
        handle?.setFrame(EdgeDockLayout.handleFrame(for: dockFrame, side: model.edgeDockSide), display: true)
        if let id = model.dockProvider, model.snapshot?.providers.contains(where: { $0.id == id }) != true {
            dismissPeek()
        }
        positionPeek()
        reconcileMonitors()
        pointerChanged(animated: false)
    }

    func closePeek() {
        dismissPeek()
        reconcileMonitors()
        pointerChanged()
    }

    private func dismissPeek() {
        guard model?.dockProvider != nil || model?.dockPeekPinned == true || peek?.isVisible == true else { return }
        peek?.orderOut(nil)
        model?.dockProvider = nil
        model?.dockPeekPinned = false
    }

    func shutdown() {
        shuttingDown = true
        setVisible(false)
    }

    func moveToPointerScreen() {
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        configureContent()
        update()
    }

    private func reveal() {
        guard !shuttingDown, model?.edgeDockEnabled == true else { return }
        setExpanded(true, animated: true)
    }

    private func present(_ id: String, pin: Bool) {
        guard let model else { return }
        if model.panelVisible {
            if !pin { return }
            openOverview?()
        }
        if !pin && model.dockPeekPinned { return }
        if model.dockProvider == id {
            if pin {
                model.dockPeekPinned.toggle()
                if model.dockPeekPinned { peek?.makeKeyAndOrderFront(nil) }
            }
            return
        }
        model.dockProvider = id
        model.dockPeekPinned = pin
        setExpanded(true, animated: true)
        if peek == nil { peek = makePanel(title: "Aims provider details") }
        guard let peek else { return }
        configurePeekContent()
        positionPeek()
        if pin { peek.makeKeyAndOrderFront(nil) } else { peek.orderFrontRegardless() }
        reconcileMonitors()
    }

    private func configurePeekContent() {
        guard let peek, let model, let id = model.dockProvider else { return }
        let maximum = max(140, (screen?.visibleFrame.height ?? 800) - 110)
        let root = AnyView(
            ProviderPeekView(
                model: model, providerID: id, maximumBodyHeight: maximum,
                close: { [weak self] in self?.closePeek() },
                togglePin: { [weak self, weak model] in
                    model?.dockPeekPinned.toggle()
                    self?.pointerChanged()
                },
                heightDidChange: { [weak self] height in
                    self?.peekHeight = height
                    self?.positionPeek()
                }
            ).id(id))
        if let peekHostingView {
            peekHostingView.rootView = root
        } else {
            let host = NSHostingView(rootView: root)
            peekHostingView = host
            peek.contentView = host
        }
    }

    private func positionPeek() {
        guard let peek, let dock, let model, let frame = (screen ?? NSScreen.main)?.visibleFrame,
            let id = model.dockProvider,
            let index = model.snapshot?.displayProviders.firstIndex(where: { $0.id == id })
        else { return }
        let size = model.edgeDockSize
        let height = min(peekHeight, frame.height - 24)
        let anchorY = dock.frame.maxY - size.contentInset - (size.rowStride - 4) / 2 - CGFloat(index) * size.rowStride
        let y = min(frame.maxY - height - 12, max(frame.minY + 12, anchorY - height / 2))
        let x =
            model.edgeDockSide == .right
            ? max(frame.minX + 12, dock.frame.minX - EdgeDockLayout.peekWidth - EdgeDockLayout.gap)
            : min(frame.maxX - EdgeDockLayout.peekWidth - 12, dock.frame.maxX + EdgeDockLayout.gap)
        peek.setFrame(NSRect(x: x, y: y, width: EdgeDockLayout.peekWidth, height: height), display: true)
    }

    private func reconcileMonitors() {
        guard !shuttingDown, let model, model.edgeDockEnabled else {
            pointerMonitor.stop()
            return
        }
        if model.edgeDockAutoHide || model.dockProvider != nil {
            pointerMonitor.start { [weak self] in self?.pointerChanged() }
        } else {
            pointerMonitor.stop()
        }
    }

    private func pointerChanged(animated: Bool = true) {
        guard !shuttingDown, let model, model.edgeDockEnabled, let dock, let handle else { return }
        let point = NSEvent.mouseLocation
        let peekFrame = peek?.isVisible == true ? peek?.frame : nil
        let inside =
            dock.frame.insetBy(dx: -14, dy: -14).contains(point)
            || peekFrame?.insetBy(dx: -14, dy: -14).contains(point) == true
        if !model.dockPeekPinned && !inside { dismissPeek() }
        let visible = EdgeDockVisibility.shouldReveal(
            autoHide: model.edgeDockAutoHide, expanded: expanded || dock.isVisible,
            pinned: model.dockPeekPinned && peek?.isVisible == true, pointer: point,
            dock: dock.frame, peek: peek?.isVisible == true ? peek?.frame : nil, activation: handle.frame)
        setExpanded(visible, animated: animated)
        reconcileMonitors()
    }

    private func setExpanded(_ visible: Bool, animated: Bool) {
        guard let dock, let handle else { return }
        guard !hasPresented || expanded != visible else { return }
        hasPresented = true
        expanded = visible
        presentationGeneration += 1
        let generation = presentationGeneration
        let keyboard = NSApp.currentEvent?.type == .keyDown || NSApp.currentEvent?.type == .keyUp
        let animate = animated && !keyboard && !NSWorkspace.shared.accessibilityDisplayShouldReduceMotion
        if visible {
            handle.orderOut(nil)
            if animate && !dock.isVisible { dock.alphaValue = 0 }
            dock.orderFrontRegardless()
            if animate {
                NSAnimationContext.runAnimationGroup { context in
                    context.duration = 0.16
                    dock.animator().alphaValue = 1
                }
            } else {
                dock.alphaValue = 1
            }
        } else {
            handle.orderFrontRegardless()
            if animate && dock.isVisible {
                NSAnimationContext.runAnimationGroup { context in
                    context.duration = 0.12
                    dock.animator().alphaValue = 0
                } completionHandler: { [weak self] in
                    DispatchQueue.main.async {
                        guard let self, self.presentationGeneration == generation else { return }
                        self.dock?.orderOut(nil)
                        self.dock?.alphaValue = 1
                    }
                }
            } else {
                dock.orderOut(nil)
                dock.alphaValue = 1
            }
        }
    }

    @objc private func screenChanged() {
        if let old = screen, !NSScreen.screens.contains(old) { screen = NSScreen.main }
        configureContent()
        configurePeekContent()
        update()
    }

    private func makePanel(title: String) -> UsagePanel {
        let panel = UsagePanel(
            contentRect: .zero, styleMask: [.borderless, .nonactivatingPanel], backing: .buffered, defer: false)
        panel.title = title
        panel.isOpaque = false
        panel.backgroundColor = .clear
        panel.hasShadow = true
        panel.isFloatingPanel = true
        panel.level = .floating
        panel.hidesOnDeactivate = false
        panel.isReleasedWhenClosed = false
        panel.acceptsMouseMovedEvents = true
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary]
        return panel
    }
}
