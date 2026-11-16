import SwiftUI

struct DockDisplayPicker: View {
    @Bindable var model: UsageModel

    var body: some View {
        LazyVGrid(columns: Array(repeating: GridItem(.fixed(116), spacing: 12), count: columnCount), spacing: 10) {
            ForEach(Array(model.dockDisplays.enumerated()), id: \.element.id) { index, display in
                displayCard(display, index: index)
            }
        }.frame(maxWidth: .infinity, alignment: .leading)
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
                Text(display.name).font(.callout.weight(selected ? .semibold : .medium)).lineLimit(2)
                    .foregroundStyle(selected ? Color.accentColor : Color.primary)
                    .multilineTextAlignment(.center)
                    .fixedSize(horizontal: false, vertical: true)
                    .frame(height: 34)
                Text("\(index + 1) · " + (index == 0 ? "Primary" : "Secondary"))
                    .font(.system(size: 10)).foregroundStyle(.secondary)
            }
            .padding(6).frame(maxWidth: .infinity)
            .contentShape(.rect(cornerRadius: 10))
        }
        .buttonStyle(.plain)
        .help(display.name + (selected ? " · Edge Dock is on this display" : " · Place Edge Dock on this display"))
        .accessibilityLabel("\(display.name), display \(index + 1), " + (index == 0 ? "primary" : "secondary"))
        .accessibilityValue(selected ? "Selected" : "Not selected")
    }
}
