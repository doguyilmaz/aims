import AppKit
import SwiftUI

/// Intercepts secondary clicks only; the SwiftUI button retains primary clicks and keyboard focus.
struct DockContextMenu: NSViewRepresentable {
    let showOptions: @MainActor (CGPoint) -> Void

    func makeNSView(context: Context) -> DockContextMenuView { DockContextMenuView() }

    func updateNSView(_ view: DockContextMenuView, context: Context) {
        view.showOptions = showOptions
    }
}
