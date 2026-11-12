import AppKit
import SwiftUI

@MainActor
final class EdgeDockController: NSObject {
    private var dock: UsagePanel?
    private var peek: UsagePanel?
    private var peekHostingView: NSHostingView<AnyView>?
    private weak var model: UsageModel?
    private var screen: NSScreen?
    private var peekHeight: CGFloat = 240
    private var globalMonitor: Any?
    private var localMonitor: Any?
    private var openOverview: (@MainActor () -> Void)?

    func start(model: UsageModel, openOverview: @escaping @MainActor () -> Void) {
        self.model = model
        self.openOverview = openOverview
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        let dock = makePanel(title: "Aims Edge Dock")
        self.dock = dock
        configureContent()
        NotificationCenter.default.addObserver(
            self, selector: #selector(screenChanged),
            name: NSApplication.didChangeScreenParametersNotification, object: nil)
        update()
        setVisible(model.edgeDockEnabled)
    }

    private func configureContent() {
        guard let dock, let model, let openOverview else { return }
        dock.contentView = NSHostingView(
            rootView: EdgeDockView(
                model: model, maximumHeight: (screen?.visibleFrame.height ?? 800) - 24,
                hoverProvider: { [weak self] id in self?.present(id, pin: false) },
                selectProvider: { [weak self] id in self?.present(id, pin: true) },
                openOverview: { [weak self] in
                    self?.closePeek()
                    openOverview()
                }))
    }

    func setVisible(_ visible: Bool) {
        if visible {
            update()
            dock?.orderFrontRegardless()
        } else {
            closePeek()
            dock?.orderOut(nil)
        }
    }

    func update() {
        guard let dock, let frame = (screen ?? NSScreen.main)?.visibleFrame else { return }
        let count = model?.snapshot?.providers.count ?? 0
        let height = min(frame.height - 24, count == 0 ? 130 : CGFloat(count) * 72 + 78)
        dock.setFrame(NSRect(x: frame.maxX - 90, y: frame.midY - height / 2, width: 78, height: height), display: true)
        if let id = model?.dockProvider, model?.snapshot?.providers.contains(where: { $0.id == id }) != true {
            closePeek()
        }
        positionPeek()
    }

    func closePeek() {
        peek?.orderOut(nil)
        model?.dockProvider = nil
        model?.dockPeekPinned = false
        if let globalMonitor {
            NSEvent.removeMonitor(globalMonitor)
            self.globalMonitor = nil
        }
        if let localMonitor {
            NSEvent.removeMonitor(localMonitor)
            self.localMonitor = nil
        }
    }

    func shutdown() {
        closePeek()
        dock?.orderOut(nil)
    }

    func moveToPointerScreen() {
        screen = NSScreen.screens.first { $0.frame.contains(NSEvent.mouseLocation) } ?? NSScreen.main
        configureContent()
        update()
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
        if peek == nil { peek = makePanel(title: "Aims provider details") }
        guard let peek else { return }
        configurePeekContent()
        positionPeek()
        if pin { peek.makeKeyAndOrderFront(nil) } else { peek.orderFrontRegardless() }
        installMonitors()
    }

    private func configurePeekContent() {
        guard let peek, let model, let id = model.dockProvider else { return }
        let maximum = max(140, (screen?.visibleFrame.height ?? 800) - 110)
        let root = AnyView(
            ProviderPeekView(
                model: model, providerID: id, maximumBodyHeight: maximum,
                close: { [weak self] in self?.closePeek() },
                togglePin: { [weak model] in model?.dockPeekPinned.toggle() },
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
        guard let peek, let dock, let frame = (screen ?? NSScreen.main)?.visibleFrame,
            let id = model?.dockProvider,
            let index = model?.snapshot?.displayProviders.firstIndex(where: { $0.id == id })
        else { return }
        let height = min(peekHeight, frame.height - 24)
        let anchorY = dock.frame.maxY - 78 - CGFloat(index) * 72
        let y = min(frame.maxY - height - 12, max(frame.minY + 12, anchorY - height / 2))
        peek.setFrame(
            NSRect(x: max(frame.minX + 12, dock.frame.minX - 462), y: y, width: 450, height: height), display: true)
    }

    private func installMonitors() {
        guard globalMonitor == nil && localMonitor == nil else { return }
        globalMonitor = NSEvent.addGlobalMonitorForEvents(matching: .mouseMoved) { [weak self] _ in
            self?.dismissIfOutside()
        }
        localMonitor = NSEvent.addLocalMonitorForEvents(matching: .mouseMoved) { [weak self] event in
            self?.dismissIfOutside()
            return event
        }
    }

    private func dismissIfOutside() {
        guard model?.dockPeekPinned == false else { return }
        let point = NSEvent.mouseLocation
        let insideDock = dock?.frame.insetBy(dx: -14, dy: -14).contains(point) == true
        let insidePeek = peek?.frame.insetBy(dx: -14, dy: -14).contains(point) == true
        if !insideDock && !insidePeek { closePeek() }
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
