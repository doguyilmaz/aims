import AppKit
import SwiftUI
import UsageCore

/// Intercepts secondary clicks only; the SwiftUI button retains primary clicks and keyboard focus.
struct DockContextMenu: NSViewRepresentable {
    let model: UsageModel
    var provider: UsageProvider?
    let openOverview: @MainActor () -> Void

    func makeNSView(context: Context) -> DockContextMenuView {
        DockContextMenuView()
    }

    func updateNSView(_ view: DockContextMenuView, context: Context) {
        view.makeMenu = { DockNativeMenu.make(model: model, provider: provider, openOverview: openOverview) }
    }
}
