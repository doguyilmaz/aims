import Foundation

enum EdgeDockVisibility {
    static func shouldReveal(
        autoHide: Bool, expanded: Bool, pinned: Bool, dockPinned: Bool = false, optionsPresented: Bool = false,
        pointer: CGPoint,
        dock: CGRect, peek: CGRect?, activation: CGRect
    ) -> Bool {
        if !autoHide || pinned || dockPinned || optionsPresented { return true }
        if !expanded { return activation.contains(pointer) }
        return dock.insetBy(dx: -14, dy: -14).contains(pointer)
            || peek?.insetBy(dx: -14, dy: -14).contains(pointer) == true
    }
}
