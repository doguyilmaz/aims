import SwiftUI

extension View {
    func usagePanelSurface<S: Shape>(in shape: S, systemPopover: Bool = false) -> some View {
        modifier(UsagePanelSurface(shape: shape, systemPopover: systemPopover))
    }
}
