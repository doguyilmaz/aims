import SwiftUI
import UsageCore

struct DockFocusPicker: View {
    @Bindable var model: UsageModel
    let provider: UsageProvider
    var title = "Usage window"
    var showsProviderMark = false

    var body: some View {
        let selected = model.dockFocus(for: provider.id)
        let reported = DockWindowFocus.options(for: provider)
        Picker(
            selection: Binding(
                get: { model.dockFocus(for: provider.id) },
                set: { model.setDockFocus($0, for: provider.id) })
        ) {
            ForEach(DockWindowFocus.options(for: provider, including: selected)) { focus in
                Text(focus.title + (reported.contains(focus) ? "" : " · Not reported")).tag(focus)
            }
        } label: {
            if showsProviderMark {
                HStack(spacing: 10) {
                    ProviderMark(id: provider.id, size: 20)
                    Text(title)
                }
            } else {
                Text(title)
            }
        }
    }
}
