import AppKit

nonisolated final class DockMenuItem: NSMenuItem {
    private let perform: @MainActor () -> Void

    @MainActor init(title: String, checked: Bool = false, perform: @escaping @MainActor () -> Void) {
        self.perform = perform
        super.init(title: title, action: #selector(invoke), keyEquivalent: "")
        target = self
        state = checked ? .on : .off
    }

    required init(coder: NSCoder) { fatalError("init(coder:) has not been implemented") }

    @MainActor @objc private func invoke() { perform() }
}
