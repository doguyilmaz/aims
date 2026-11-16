import SwiftUI

struct DockDisplayPicker: View {
    @Bindable var model: UsageModel

    var body: some View {
        Picker("Display", selection: $model.edgeDockDisplay) {
            Text("Primary display").tag(DockDisplaySelection.primary)
            ForEach(Array(model.dockDisplays.enumerated()), id: \.element.id) { index, display in
                Text("\(index + 1) · \(display.name)").tag(DockDisplaySelection.display(display.id))
            }
            if model.edgeDockDisplay != .primary && !model.edgeDockDisplay.isConnected(in: model.dockDisplays) {
                Text("Saved display · Disconnected").tag(model.edgeDockDisplay)
            }
        }
        .help("The Dock stays on the chosen display. Its menu-bar popover and overview open independently.")
    }
}
