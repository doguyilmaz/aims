import SwiftUI

extension View {
    @ViewBuilder func settingsPickerSizing(width: CGFloat) -> some View {
        if #available(macOS 26, *) {
            buttonSizing(.flexible).frame(width: width)
        } else {
            frame(width: width)
        }
    }
}
