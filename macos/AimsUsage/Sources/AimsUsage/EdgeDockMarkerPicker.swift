import SwiftUI

struct EdgeDockMarkerPicker: View {
    @Bindable var model: UsageModel

    var body: some View {
        HStack(spacing: 6) {
            ForEach(EdgeDockMarkerStyle.allCases) { style in
                let selected = model.edgeDockMarkerStyle == style
                Button {
                    if !selected { model.edgeDockMarkerStyle = style }
                } label: {
                    HStack(spacing: 5) {
                        EdgeDockMarkerView(style: style, side: model.edgeDockSide, size: .mini)
                            .scaleEffect(0.65).frame(width: 18, height: 22)
                        Text(style.title).font(.caption.weight(selected ? .semibold : .regular))
                            .lineLimit(1)
                    }
                    .frame(width: 66, height: 34)
                    .background(
                        selected ? Color.accentColor.opacity(0.1) : Color.primary.opacity(0.03),
                        in: RoundedRectangle(cornerRadius: 8)
                    )
                    .overlay {
                        RoundedRectangle(cornerRadius: 8)
                            .stroke(selected ? Color.accentColor : .primary.opacity(0.1), lineWidth: 1)
                    }
                    .contentShape(.rect(cornerRadius: 8))
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Hidden Dock marker: \(style.title)")
                .accessibilityValue(selected ? "Selected" : "Not selected")
                .accessibilityAddTraits(selected ? .isSelected : [])
                .help(style.hint)
            }
        }
    }
}
