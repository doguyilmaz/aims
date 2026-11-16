import SwiftUI

struct EdgeDockSettings: View {
    @Bindable var model: UsageModel

    var body: some View {
        Form {
            Section("Display") {
                DockDisplayPicker(model: model)
                Toggle("Follow primary display", isOn: followPrimary)
                    .help("Follow the primary display chosen in macOS. Select a monitor to keep the Dock there.")
                if !model.edgeDockDisplay.isConnected(in: model.dockDisplays) {
                    Text("Selected display is disconnected. Using the primary display until it reconnects.")
                        .foregroundStyle(.secondary)
                }
            }
            Section {
                LabeledContent("Screen edge") {
                    Picker("Screen edge", selection: $model.edgeDockSide) {
                        ForEach(EdgeDockSide.allCases) { Text($0.title).tag($0) }
                    }.pickerStyle(.segmented).labelsHidden().settingsPickerSizing(width: 220)
                }
                LabeledContent("Dock size") {
                    Picker("Dock size", selection: $model.edgeDockSize) {
                        ForEach(EdgeDockSize.allCases) { Text($0.title).tag($0) }
                    }.pickerStyle(.segmented).labelsHidden().settingsPickerSizing(width: 220)
                }
                LabeledContent("Vertical position") {
                    HStack(spacing: 10) {
                        Slider(value: $model.edgeDockPosition, in: 0...1) {
                            Text("Vertical position")
                        } minimumValueLabel: {
                            Image(systemName: "arrow.up.to.line")
                        } maximumValueLabel: {
                            Image(systemName: "arrow.down.to.line")
                        }
                        .labelsHidden()
                        .accessibilityValue("\(Int(model.edgeDockPosition * 100)) percent from top")
                        Button("Center") { model.edgeDockPosition = 0.5 }
                            .buttonStyle(.borderless).accessibilityLabel("Center Edge Dock")
                    }.frame(width: 260)
                }
                Toggle("Automatically hide", isOn: $model.edgeDockAutoHide)
                Toggle("Pin Edge Dock", isOn: $model.edgeDockPinned)
            } header: {
                Text("Placement & behavior")
            } footer: {
                Text(
                    model.edgeDockPinned
                        ? "Pinned: the Dock stays visible."
                        : model.edgeDockAutoHide
                            ? "Hover over its edge marker to reveal the Dock." : "The Dock stays visible."
                )
            }
        }.formStyle(.grouped).toggleStyle(.switch)
    }

    private var followPrimary: Binding<Bool> {
        Binding(
            get: { model.edgeDockDisplay == .primary },
            set: { follow in
                if follow {
                    model.edgeDockDisplay = .primary
                } else if let display = model.edgeDockDisplay.resolve(in: model.dockDisplays) {
                    model.edgeDockDisplay = .display(display.id)
                }
            })
    }
}
