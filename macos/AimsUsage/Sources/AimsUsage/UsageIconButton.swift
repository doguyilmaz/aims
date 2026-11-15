import AppKit
import SwiftUI

struct UsageIconButton: View {
    let title: String
    let icon: UsageControlIcon
    var selected = false
    var busy = false
    var tint: Color?
    var width: CGFloat = 24
    var height: CGFloat = 24
    var role: ButtonRole?
    let action: @MainActor () -> Void
    @State private var hovered = false

    var body: some View {
        Button(role: role) {
            let panel = NSApp.currentEvent?.window as? UsagePanel
            action()
            panel?.clearPointerFocus()
        } label: {
            Group {
                if busy {
                    ProgressView().controlSize(.mini)
                } else {
                    UsageControlSymbol(icon: icon)
                }
            }.frame(width: 16, height: 16)
        }
        .buttonStyle(
            UsageIconButtonStyle(selected: selected, hovered: hovered, tint: tint, width: width, height: height)
        )
        .onHover { hovered = $0 }
        .help(title).accessibilityLabel(title)
        .accessibilityAddTraits(selected ? .isSelected : [])
    }
}
