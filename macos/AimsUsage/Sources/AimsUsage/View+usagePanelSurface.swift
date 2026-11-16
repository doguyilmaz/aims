import SwiftUI

extension View {
    func usagePanelSurface<S: Shape>(in shape: S) -> some View {
        modifier(UsagePanelSurface(shape: shape))
    }
}
