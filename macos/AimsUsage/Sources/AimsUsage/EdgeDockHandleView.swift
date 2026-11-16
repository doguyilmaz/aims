import SwiftUI

struct EdgeDockHandleView: View {
    let side: EdgeDockSide
    let style: EdgeDockMarkerStyle
    let size: EdgeDockSize
    let reveal: @MainActor () -> Void

    var body: some View {
        Button(action: reveal) {
            EdgeDockMarkerView(style: style, side: side, size: size)
                .frame(width: style.activationWidth(for: size), alignment: side == .right ? .trailing : .leading)
                .frame(maxHeight: .infinity)
                .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .onHover { if $0 { reveal() } }
        .help("Hover at this edge to show Edge Dock")
        .accessibilityLabel("Reveal Edge Dock")
    }
}
