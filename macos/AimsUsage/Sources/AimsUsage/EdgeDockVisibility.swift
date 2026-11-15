import Foundation

enum EdgeDockVisibility {
    static func shouldReveal(
        autoHide: Bool, expanded: Bool, pinned: Bool, pointer: CGPoint,
        dock: CGRect, peek: CGRect?, activation: CGRect
    ) -> Bool {
        if !autoHide || pinned { return true }
        if !expanded { return activation.contains(pointer) }
        return dock.insetBy(dx: -14, dy: -14).contains(pointer)
            || peek?.insetBy(dx: -14, dy: -14).contains(pointer) == true
    }
}
