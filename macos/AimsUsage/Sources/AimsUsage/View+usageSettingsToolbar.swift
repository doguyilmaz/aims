import SwiftUI

extension View {
    @ViewBuilder func usageSettingsToolbar() -> some View {
        if #available(macOS 15.0.0, *) {
            toolbarVisibility(.hidden, for: .windowToolbar)
        } else {
            self
        }
    }
}
