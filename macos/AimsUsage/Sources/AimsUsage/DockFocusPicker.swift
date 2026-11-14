import SwiftUI
import UsageCore

struct DockFocusPicker: View {
    @Bindable var model: UsageModel
    let provider: UsageProvider
    var title = "Usage window"

    var body: some View {
        let selected = model.dockFocus(for: provider.id)
        let reported = DockWindowFocus.options(for: provider)
        Picker(
            title,
            selection: Binding(
                get: { model.dockFocus(for: provider.id) },
                set: { model.setDockFocus($0, for: provider.id) })
        ) {
            ForEach(DockWindowFocus.options(for: provider, including: selected)) { focus in
                Text(focus.title + (reported.contains(focus) ? "" : " · Not reported")).tag(focus)
            }
        }
    }
}
