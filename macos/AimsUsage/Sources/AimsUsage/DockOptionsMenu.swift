import SwiftUI
import UsageCore

struct DockOptionsMenu: View {
    @Bindable var model: UsageModel
    var provider: UsageProvider?
    let openOverview: @MainActor () -> Void

    var body: some View {
        if let provider {
            DockFocusPicker(model: model, provider: provider).pickerStyle(.inline)
        } else if let providers = model.snapshot?.displayProviders {
            Menu("Usage windows") {
                ForEach(providers) { provider in
                    Menu(ProviderStyle.title(provider)) {
                        DockFocusPicker(model: model, provider: provider).pickerStyle(.inline)
                    }
                }
            }
        }
        Picker("Percentages", selection: $model.edgeDockValueMode) {
            ForEach(DockValueMode.allCases) { mode in Text(mode.title).tag(mode) }
        }.pickerStyle(.inline)
        Divider()
        Toggle("Pin Edge Dock", isOn: $model.edgeDockPinned)
        Button("Open overview", action: openOverview)
        Button("Hide Edge Dock") { model.edgeDockEnabled = false }
    }
}
