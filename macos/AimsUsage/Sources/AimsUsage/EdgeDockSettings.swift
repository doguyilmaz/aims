import SwiftUI

struct EdgeDockSettings: View {
    @Bindable var model: UsageModel

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                GroupBox("Display") { DockDisplayPicker(model: model).padding(8) }
                GroupBox {
                    HStack(alignment: .top, spacing: 16) {
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Placement").font(.system(size: 12, weight: .semibold)).accessibilityAddTraits(
                                .isHeader)
                            placement
                        }.frame(maxWidth: .infinity, alignment: .topLeading)
                        Divider()
                        VStack(alignment: .leading, spacing: 12) {
                            Text("Readings").font(.system(size: 12, weight: .semibold)).accessibilityAddTraits(
                                .isHeader)
                            readings
                        }.frame(maxWidth: .infinity, alignment: .topLeading)
                    }.fixedSize(horizontal: false, vertical: true).padding(10)
                }
            }.padding(20)
        }
    }

    private var placement: some View {
        VStack(alignment: .leading, spacing: 12) {
            LabeledContent("Screen edge") {
                Picker("Screen edge", selection: $model.edgeDockSide) {
                    ForEach(EdgeDockSide.allCases) { Text($0.title).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().frame(width: 124)
            }
            LabeledContent("Dock size") {
                Picker("Dock size", selection: $model.edgeDockSize) {
                    ForEach(EdgeDockSize.allCases) { Text($0.title).tag($0) }
                }.pickerStyle(.segmented).labelsHidden().frame(width: 124)
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
            LabeledContent("Automatically hide") {
                Toggle("Automatically hide", isOn: $model.edgeDockAutoHide).labelsHidden()
                    .toggleStyle(.switch).controlSize(.small)
            }
            LabeledContent("Pin Edge Dock") {
                Toggle("Pin Edge Dock", isOn: $model.edgeDockPinned).labelsHidden()
                    .toggleStyle(.switch).controlSize(.small)
            }
            Text(
                model.edgeDockPinned
                    ? "Pinned: the Dock stays visible."
                    : model.edgeDockAutoHide
                        ? "Hover over its edge marker to reveal the Dock." : "The Dock stays visible."
            ).font(.caption).foregroundStyle(.secondary)
        }.font(.system(size: 12))
    }

    private var readings: some View {
        VStack(alignment: .leading, spacing: 12) {
            Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 10) {
                GridRow {
                    Text("Percentages")
                    Picker("Percentages", selection: $model.edgeDockValueMode) {
                        ForEach(DockValueMode.allCases) { Text($0.title).tag($0) }
                    }.labelsHidden().frame(width: 110)
                }
                Divider().gridCellColumns(2)
                if let providers = model.snapshot?.displayProviders {
                    ForEach(providers) { provider in
                        GridRow {
                            Text(ProviderStyle.title(provider)).lineLimit(1)
                            DockFocusPicker(model: model, provider: provider, title: ProviderStyle.title(provider))
                                .labelsHidden().frame(width: 110)
                        }
                    }
                }
            }
            Text("Choose the limit window for each provider. Select accounts in hover details or Dock options.")
                .font(.caption).foregroundStyle(.secondary)
        }.font(.system(size: 12)).controlSize(.small)
    }
}
