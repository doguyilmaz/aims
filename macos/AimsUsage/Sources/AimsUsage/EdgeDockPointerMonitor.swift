import AppKit

@MainActor
final class EdgeDockPointerMonitor {
    private var global: Any?
    private var local: Any?

    func start(_ changed: @escaping @MainActor () -> Void) {
        guard global == nil && local == nil else { return }
        global = NSEvent.addGlobalMonitorForEvents(matching: .mouseMoved) { _ in changed() }
        local = NSEvent.addLocalMonitorForEvents(matching: .mouseMoved) { event in
            changed()
            return event
        }
    }

    func stop() {
        if let global { NSEvent.removeMonitor(global) }
        if let local { NSEvent.removeMonitor(local) }
        global = nil
        local = nil
    }
}
