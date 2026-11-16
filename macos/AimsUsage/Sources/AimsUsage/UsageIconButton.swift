import AppKit
import SwiftUI

struct UsageIconButton: View {
    let title: String
    let icon: UsageControlIcon
    var selected = false
    var busy = false
    var tint: Color?
    var symbolSize: CGFloat? = nil
    var width: CGFloat = 24
    var height: CGFloat = 24
    var role: ButtonRole?
    let action: @MainActor () -> Void
    @State private var hovered = false

    var body: some View {
        Button(role: role) {
            let window = NSApp.currentEvent?.window
            let pointerInitiated = UsagePointerFocus.isPointerEvent
            action()
            UsagePointerFocus.clear(in: window, pointerInitiated: pointerInitiated)
        } label: {
            Group {
                if busy {
                    ProgressView().controlSize(.mini)
                } else {
                    UsageControlSymbol(icon: icon, fittedSize: symbolSize)
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
