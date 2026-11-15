import AppKit
import SwiftUI

struct UsageStyleControl: View {
    @Binding var selection: UsageDisplayStyle
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Namespace private var selectionPill
    @FocusState private var focusedStyle: UsageDisplayStyle?
    @State private var keyboardFocusVisible = false

    var body: some View {
        HStack(spacing: 2) {
            ForEach(UsageDisplayStyle.allCases) { style in
                Button {
                    let keyboard = NSApp.currentEvent?.type == .keyDown || NSApp.currentEvent?.type == .keyUp
                    keyboardFocusVisible = keyboard
                    withAnimation(reduceMotion || keyboard ? nil : .easeOut(duration: 0.18)) { selection = style }
                    focusedStyle = style
                } label: {
                    Text(style.title).font(.system(size: 12, weight: .medium))
                        .foregroundStyle(selection == style ? .primary : .secondary)
                        .frame(width: 60, height: 28)
                        .background {
                            if selection == style {
                                RoundedRectangle(cornerRadius: 7).fill(.cyan.opacity(0.16))
                                    .matchedGeometryEffect(id: "selected", in: selectionPill)
                            }
                        }.contentShape(Rectangle())
                }
                .buttonStyle(.plain).focused($focusedStyle, equals: style).focusEffectDisabled()
                .overlay(
                    RoundedRectangle(cornerRadius: 7).strokeBorder(
                        .cyan.opacity(keyboardFocusVisible && focusedStyle == style ? 0.45 : 0), lineWidth: 1)
                )
                .accessibilityLabel(style.title)
                .accessibilityAddTraits(selection == style ? .isSelected : [])
            }
        }
        .padding(3).background(.primary.opacity(0.035), in: RoundedRectangle(cornerRadius: 10))
        .accessibilityElement(children: .contain).accessibilityLabel("Display style")
        .onChange(of: focusedStyle) {
            if NSApp.currentEvent?.type == .keyDown || NSApp.currentEvent?.type == .keyUp {
                keyboardFocusVisible = true
            }
        }
    }
}
