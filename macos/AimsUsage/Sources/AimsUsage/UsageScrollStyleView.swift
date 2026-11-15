import AppKit

// A noninteractive background marker for one owned window, including native
// Form/TextEditor scroll views that SwiftUI does not expose directly.
final class UsageScrollStyleView: NSView {
    var slim = false

    override func hitTest(_ point: NSPoint) -> NSView? { nil }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        NotificationCenter.default.removeObserver(
            self, name: NSScroller.preferredScrollerStyleDidChangeNotification, object: nil)
        if window != nil {
            NotificationCenter.default.addObserver(
                self, selector: #selector(preferredStyleChanged),
                name: NSScroller.preferredScrollerStyleDidChangeNotification, object: nil)
        }
        configureScrollViews()
    }

    override func layout() {
        super.layout()
        configureScrollViews()
    }

    func configureScrollViews() {
        guard let root = window?.contentView else { return }
        let style = slim ? NSScroller.Style.overlay : NSScroller.preferredScrollerStyle
        let size = slim ? NSControl.ControlSize.small : .regular
        var pending = [root]
        while let view = pending.popLast() {
            if let scroll = view as? NSScrollView {
                if scroll.scrollerStyle != style { scroll.scrollerStyle = style }
                for scroller in [scroll.verticalScroller, scroll.horizontalScroller].compactMap({ $0 }) {
                    if scroller.controlSize != size { scroller.controlSize = size }
                }
            }
            pending.append(contentsOf: view.subviews)
        }
    }

    @objc nonisolated private func preferredStyleChanged(_ notification: Notification) {
        DispatchQueue.main.async { [weak self] in self?.configureScrollViews() }
    }
}
