import AppKit
import Observation

@MainActor
final class UsageStatusItemController: NSObject {
    private var item: NSStatusItem?
    private weak var model: UsageModel?
    private var open: (@MainActor () -> Void)?

    var anchor: UsageOverviewAnchor? {
        guard let button = item?.button, button.window != nil else { return nil }
        return UsageOverviewAnchor(view: button, rect: button.bounds, edge: .minY)
    }

    func start(model: UsageModel, open: @escaping @MainActor () -> Void) {
        self.model = model
        self.open = open
        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        // Keep the position saved by the former MenuBarExtra.
        item.autosaveName = "Item-0"
        self.item = item
        if let button = item.button {
            button.image = AimsBrandMark.menuImage
            button.imagePosition = .imageLeading
            button.font = .monospacedDigitSystemFont(ofSize: NSFont.menuBarFont(ofSize: 0).pointSize, weight: .regular)
            button.target = self
            button.action = #selector(clicked)
            button.toolTip = "Aims Usage"
            button.setAccessibilityIdentifier("aims.overview.status-item")
        }
        observeLabel()
    }

    func stop() {
        guard let item else { return }
        NSStatusBar.system.removeStatusItem(item)
        self.item = nil
        open = nil
    }

    @objc private func clicked() { open?() }

    private func observeLabel() {
        guard let model, let button = item?.button else { return }
        withObservationTracking {
            let summary = model.menuSummary
            button.title = summary.map { " " + $0 } ?? ""
            button.highlight(model.overviewPresentation.isVisible)
            button.setAccessibilityLabel("Aims Usage" + (summary.map { ", " + $0 } ?? ""))
        } onChange: { [weak self] in
            // Observation fires before the mutation completes; AppKit updates belong to the main run loop.
            DispatchQueue.main.async { self?.observeLabel() }
        }
    }
}
