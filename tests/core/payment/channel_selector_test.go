package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/huwenlong92/sdkit/core/payment"
)

func TestStaticChannelSelectorCanBeReloaded(t *testing.T) {
	selector, err := payment.NewStaticChannelSelector([]payment.ChannelBinding{
		{
			Key:         "school_a_p",
			Provider:    payment.ProviderWechat,
			Channel:     payment.ChannelWechatMiniProgram,
			MerchantKey: "school_a_wechat",
		},
	})
	if err != nil {
		t.Fatalf("new selector: %v", err)
	}

	if err := selector.Reload([]payment.ChannelBinding{
		{
			Key:         "school_b_p",
			Provider:    payment.ProviderAlipay,
			Channel:     payment.ChannelAlipayPage,
			MerchantKey: "school_b_alipay",
		},
	}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	_, err = selector.SelectPaymentChannel(context.Background(), payment.ChannelSelectionRequest{
		MerchantKey: "school_a_p",
	})
	if !errors.Is(err, payment.ErrAdapterNotFound) {
		t.Fatalf("old key err = %v, want ErrAdapterNotFound", err)
	}

	selection, err := selector.SelectPaymentChannel(context.Background(), payment.ChannelSelectionRequest{
		MerchantKey: "school_b_p",
	})
	if err != nil {
		t.Fatalf("select new key: %v", err)
	}
	if selection.Provider != payment.ProviderAlipay ||
		selection.Channel != payment.ChannelAlipayPage ||
		selection.MerchantKey != "school_b_alipay" {
		t.Fatalf("selection = %+v", selection)
	}
}
