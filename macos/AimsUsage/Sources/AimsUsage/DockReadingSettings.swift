import SwiftUI

struct DockReadingSettings: View {
    @Bindable var model: UsageModel

    var body: some View {
        Form {
            Section {
                HStack(alignment: .center) {
                    Text("Show percentages as").accessibilityHidden(true)
                    Spacer()
                    Picker("Show percentages as", selection: $model.edgeDockValueMode) {
                        ForEach(DockValueMode.allCases) { Text($0.title).tag($0) }
                    }.labelsHidden().fixedSize()
                }
            } header: {
                Text("Percentage display")
            } footer: {
                Text("Used shows consumption. Remaining shows the allowance left in the selected limit window.")
            }
            Section {
                if let providers = model.snapshot?.displayProviders, !providers.isEmpty {
                    ForEach(providers) { provider in
                        DockFocusPicker(
                            model: model, provider: provider, title: ProviderStyle.title(provider),
                            showsProviderMark: true)
                    }
                } else {
                    Text("Provider choices appear after usage has loaded.")
                        .foregroundStyle(.secondary)
                }
            } header: {
                Text("Limit window by provider")
            } footer: {
                Text(
                    "Automatic chooses the most constrained current window. Select accounts in provider details or Dock options."
                )
            }
        }.formStyle(.grouped)
    }
}
