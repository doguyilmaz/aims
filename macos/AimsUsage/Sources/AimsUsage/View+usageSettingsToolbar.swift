import SwiftUI

extension View {
    @ViewBuilder func usageSettingsToolbar() -> some View {
        if #available(macOS 15.0.0, *) {
            toolbarVisibility(.visible, for: .windowToolbar)
        } else {
            self
        }
    }
}
