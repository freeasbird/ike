package protocol

import (
	"fmt"
)

// Transforms store the configured crypto suite
type Transforms map[TransformType]*SaTransform

func (t *Transforms) AsProposal(pID ProtocolID) (prop Proposals) {
	prop = append(prop, &SaProposal{
		ProtocolID:   pID,
		SaTransforms: t.AsList(),
	})
	return
}

// AsList converts transforms to flat list
func (t Transforms) AsList() (trs []*SaTransform) {
	for _, trsVal := range t {
		trs = append(trs, trsVal)
	}
	return
}

// Within checks if the configured set of transforms occurs within list of proposed transforms
func (t Transforms) Within(proposals []*SaTransform) error {
	listHas := func(trsList []*SaTransform, target *SaTransform) error {
		for _, tr := range trsList {
			if target.IsEqual(tr) {
				return nil
			}
		}
		return fmt.Errorf("%v: does not match", target.Transform)
	}
	for _, proposal := range t {
		if err := listHas(proposals, proposal); err != nil {
			return err
		}
	}
	return nil
}

func (t Transforms) GetType(ty TransformType) *Transform {
	trs, ok := t[ty]
	if !ok {
		return nil
	}
	return &trs.Transform
}

// IkeTransform builds a IKE cipher suite
func IkeTransform(encr EncrTransformId, keyBits uint16, auth AuthTransformId, prf PrfTransformId, dh DhTransformId) Transforms {
	return Transforms{
		TRANSFORM_TYPE_ENCR: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_ENCR,
				TransformId: uint16(encr),
			},
			KeyLength: keyBits,
		},
		TRANSFORM_TYPE_INTEG: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_INTEG,
				TransformId: uint16(auth),
			},
		},
		TRANSFORM_TYPE_PRF: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_PRF,
				TransformId: uint16(prf),
			},
		},
		TRANSFORM_TYPE_DH: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_DH,
				TransformId: uint16(dh),
			},
			IsLast: true,
		},
	}
}

// EspTransform builds an ESP cipher suite
func EspTransform(encr EncrTransformId, keyBits uint16, auth AuthTransformId, esn EsnTransformId) Transforms {
	return Transforms{
		TRANSFORM_TYPE_ENCR: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_ENCR,
				TransformId: uint16(encr),
			}, KeyLength: keyBits},
		TRANSFORM_TYPE_INTEG: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_INTEG,
				TransformId: uint16(auth),
			},
		},
		TRANSFORM_TYPE_ESN: &SaTransform{
			Transform: Transform{
				Type:        TRANSFORM_TYPE_ESN,
				TransformId: uint16(esn),
			},
			IsLast: true,
		},
	}
}
