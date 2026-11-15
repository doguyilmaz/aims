import SwiftUI

struct DockOptionButton: View {
    let title: String
    var selected = false
    let action: @MainActor () -> Void
    @State private var hovered = false

    var body: some View {
        Button(action: action) {
            HStack(spacing: 8) {
                Image(systemName: "checkmark").font(.system(size: 10, weight: .semibold))
                    .opacity(selected ? 1 : 0).frame(width: 12)
                Text(title).multilineTextAlignment(.leading).frame(maxWidth: .infinity, alignment: .leading)
            }
            .font(.system(size: 13)).padding(.horizontal, 8).padding(.vertical, 5)
            .background(hovered ? Color.accentColor.opacity(0.18) : .clear, in: RoundedRectangle(cornerRadius: 6))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain).onHover { hovered = $0 }
        .accessibilityLabel(title).accessibilityValue(selected ? "Selected" : "")
    }
}
