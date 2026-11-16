import SwiftUI

struct DockDisplayPicker: View {
    @Bindable var model: UsageModel

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            LazyVGrid(columns: Array(repeating: GridItem(.fixed(116), spacing: 12), count: columnCount), spacing: 10) {
                ForEach(Array(model.dockDisplays.enumerated()), id: \.element.id) { index, display in
                    displayCard(display, index: index)
                }
            }.frame(maxWidth: .infinity)
            Toggle(
                "Follow primary display",
                isOn: Binding(
                    get: { model.edgeDockDisplay == .primary },
                    set: { follow in
                        if follow {
                            model.edgeDockDisplay = .primary
                        } else if let display = model.edgeDockDisplay.resolve(in: model.dockDisplays) {
                            model.edgeDockDisplay = .display(display.id)
                        }
                    })
            ).toggleStyle(.switch).controlSize(.mini)
                .help("Follow the primary display chosen in macOS. Click a monitor card to stay on that display.")
            if !model.edgeDockDisplay.isConnected(in: model.dockDisplays) {
                Text("Selected display is disconnected. Using the primary display until it reconnects.")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    private var columnCount: Int { max(1, min(4, model.dockDisplays.count)) }

    private func displayCard(_ display: DockDisplay, index: Int) -> some View {
        let selected = model.edgeDockDisplay.resolve(in: model.dockDisplays)?.id == display.id
        return Button {
            model.edgeDockDisplay = .display(display.id)
        } label: {
            VStack(spacing: 4) {
                DockDisplayPreview(
                    display: display, selected: selected, side: model.edgeDockSide, size: model.edgeDockSize,
                    position: model.edgeDockPosition, providerCount: model.snapshot?.providers.count ?? 6
                ).frame(height: 48)
                HStack(spacing: 5) {
                    Text(display.name).font(.system(size: 11, weight: .medium)).lineLimit(1)
                    if selected { Image(systemName: "checkmark.circle.fill").foregroundStyle(Color.accentColor) }
                }
                Text("\(index + 1)" + (index == 0 ? " · Primary" : ""))
                    .font(.system(size: 10)).foregroundStyle(.secondary)
            }
            .padding(8).frame(maxWidth: .infinity)
            .background(
                selected ? Color.accentColor.opacity(0.06) : .clear, in: .rect(cornerRadius: 8)
            )
            .overlay {
                RoundedRectangle(cornerRadius: 8).stroke(
                    selected ? Color.accentColor.opacity(0.6) : .clear, lineWidth: 1)
            }
            .contentShape(.rect(cornerRadius: 10))
        }
        .buttonStyle(.plain)
        .help(display.name + " · Preview of the Edge Dock position")
        .accessibilityLabel("\(display.name), display \(index + 1)" + (index == 0 ? ", primary" : ""))
        .accessibilityValue(selected ? "Selected" : "Not selected")
    }
}
