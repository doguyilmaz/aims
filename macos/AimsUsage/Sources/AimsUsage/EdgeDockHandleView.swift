import SwiftUI

struct EdgeDockHandleView: View {
    let side: EdgeDockSide
    let reveal: @MainActor () -> Void

    var body: some View {
        Button(action: reveal) {
            Capsule().fill(.gray.opacity(0.75)).frame(width: 3, height: 44)
                .frame(width: EdgeDockLayout.handleWidth, alignment: side == .right ? .trailing : .leading)
                .frame(maxHeight: .infinity)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { if $0 { reveal() } }
        .help("Hover at this edge to show Edge Dock")
        .accessibilityLabel("Reveal Edge Dock")
    }
}
