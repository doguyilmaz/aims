import SwiftUI

struct UsageIconButtonStyle: ButtonStyle {
    var selected = false
    var hovered = false
    var tint: Color?
    var width: CGFloat = 24
    var height: CGFloat = 24
    @Environment(\.isEnabled) private var enabled

    func makeBody(configuration: Configuration) -> some View {
        let engaged = enabled && (hovered || configuration.isPressed)
        let foreground = tint ?? (selected ? Color.accentColor : engaged ? .primary : .secondary)
        let opacity = !enabled ? 0 : configuration.isPressed ? 0.14 : hovered ? 0.08 : selected ? 0.05 : 0
        configuration.label
            .foregroundStyle(foreground)
            .frame(width: width, height: height)
            .background(Color.primary.opacity(opacity), in: RoundedRectangle(cornerRadius: 6))
            .contentShape(RoundedRectangle(cornerRadius: 6))
            .opacity(enabled ? 1 : 0.4)
    }
}
