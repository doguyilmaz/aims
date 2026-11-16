import SwiftUI

struct EdgeDockSettings: View {
    @Bindable var model: UsageModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                GroupBox("Display") { DockDisplayPicker(model: model).padding(8) }
                HStack(alignment: .top, spacing: 16) {
                    GroupBox("Placement") { placement.padding(8) }.frame(maxWidth: .infinity)
                    GroupBox("Readings") { readings.padding(8) }.frame(maxWidth: .infinity)
                }
            }.padding(.horizontal, 20).padding(.bottom, 20)
        }
    }

    private var placement: some View {
        VStack(alignment: .leading, spacing: 14) {
            VStack(alignment: .leading, spacing: 5) {
                Text("Screen edge").font(.caption).foregroundStyle(.secondary)
                Picker("Screen edge", selection: $model.edgeDockSide) {
                    ForEach(EdgeDockSide.allCases) { Text($0.title).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().frame(maxWidth: .infinity)
            }
            VStack(alignment: .leading, spacing: 5) {
                Text("Dock size").font(.caption).foregroundStyle(.secondary)
                Picker("Dock size", selection: $model.edgeDockSize) {
                    ForEach(EdgeDockSize.allCases) { Text($0.title).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().frame(maxWidth: .infinity)
            }
            VStack(alignment: .leading, spacing: 5) {
                HStack {
                    Text("Vertical position").font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button("Center") { model.edgeDockPosition = 0.5 }
                        .controlSize(.mini).accessibilityLabel("Center Edge Dock")
                }
                HStack {
                    Text("Top").font(.system(size: 10)).foregroundStyle(.secondary)
                    Slider(value: $model.edgeDockPosition, in: 0...1)
                        .accessibilityLabel("Edge Dock vertical position")
                        .accessibilityValue("\(Int(model.edgeDockPosition * 100)) percent from top")
                    Text("Bottom").font(.system(size: 10)).foregroundStyle(.secondary)
                }
            }
            Toggle("Automatically hide", isOn: $model.edgeDockAutoHide)
                .toggleStyle(.switch).controlSize(.small)
            Toggle("Pin Edge Dock", isOn: $model.edgeDockPinned)
                .toggleStyle(.switch).controlSize(.small)
            Text(
                model.edgeDockPinned
                    ? "Pinned: the Dock stays visible."
                    : model.edgeDockAutoHide
                        ? "Hover over its edge marker to reveal the Dock." : "The Dock stays visible."
            ).font(.caption).foregroundStyle(.secondary)
        }
    }

    private var readings: some View {
        VStack(alignment: .leading, spacing: 14) {
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 14) {
                GridRow {
                    Text("Percentages")
                    Picker("Percentages", selection: $model.edgeDockValueMode) {
                        ForEach(DockValueMode.allCases) { Text($0.title).tag($0) }
                    }.labelsHidden().frame(maxWidth: .infinity, alignment: .trailing)
                }
                Divider().gridCellColumns(2)
                if let providers = model.snapshot?.displayProviders {
                    ForEach(providers) { provider in
                        GridRow {
                            Text(ProviderStyle.title(provider)).lineLimit(1)
                            DockFocusPicker(model: model, provider: provider, title: ProviderStyle.title(provider))
                                .labelsHidden().frame(maxWidth: .infinity, alignment: .trailing)
                        }
                    }
                }
            }
            Text("Choose the limit window for each provider. Select accounts in hover details or Dock options.")
                .font(.caption).foregroundStyle(.secondary)
        }.controlSize(.small)
    }
}
