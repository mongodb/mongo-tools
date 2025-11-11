package option

// TODO: shyam - unit tests delete or fix

//
//import (
//	"encoding/json"
//	"testing"
//
//	"github.com/stretchr/testify/suite"
//	"go.mongodb.org/mongo-driver/v2/bson"
//)
//
//type mySuite struct {
//	suite.Suite
//}
//
//func TestUnitTestSuite(t *testing.T) {
//	suite.Run(t, &mySuite{})
//}
//
//func (s *mySuite) Test_Option_BSON() {
//	type MyType struct {
//		IsNone          Option[int]
//		IsNoneOmitEmpty Option[int] `bson:",omitempty"`
//		IsSome          Option[bool]
//	}
//
//	type MyTypePtrs struct {
//		IsNone          *int
//		IsNoneOmitEmpty *int `bson:",omitempty"`
//		IsSome          *bool
//	}
//
//	s.Run(
//		"marshal pointer, unmarshal Option",
//		func() {
//
//			bytes, err := bson.Marshal(MyTypePtrs{
//				IsNoneOmitEmpty: pointerTo(234),
//				IsSome:          pointerTo(false),
//			})
//			s.Require().NoError(err)
//
//			rt := MyType{}
//			s.Require().NoError(bson.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				MyType{
//					IsNoneOmitEmpty: Some(234),
//					IsSome:          Some(false),
//				},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"marshal Option, unmarshal pointer",
//		func() {
//
//			bytes, err := bson.Marshal(MyType{
//				IsNoneOmitEmpty: Some(234),
//				IsSome:          Some(false),
//			})
//			s.Require().NoError(err)
//
//			rt := MyTypePtrs{}
//			s.Require().NoError(bson.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				MyTypePtrs{
//					IsNoneOmitEmpty: pointerTo(234),
//					IsSome:          pointerTo(false),
//				},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"round-trip bson.D",
//		func() {
//			simpleDoc := bson.D{
//				{"a", None[int]()},
//				{"b", Some(123)},
//			}
//
//			bytes, err := bson.Marshal(simpleDoc)
//			s.Require().NoError(err)
//
//			rt := bson.D{}
//			s.Require().NoError(bson.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				bson.D{{"a", nil}, {"b", int32(123)}},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"round-trip struct",
//		func() {
//			myThing := MyType{None[int](), None[int](), Some(true)}
//
//			bytes, err := bson.Marshal(&myThing)
//			s.Require().NoError(err)
//
//			// Unmarshal to a bson.D to test `omitempty`.
//			rtDoc := bson.D{}
//			s.Require().NoError(bson.Unmarshal(bytes, &rtDoc))
//
//			keys := make([]string, 0)
//			for _, el := range rtDoc {
//				keys = append(keys, el.Key)
//			}
//
//			s.Assert().ElementsMatch(
//				[]string{"isnone", "issome"},
//				keys,
//			)
//
//			rtStruct := MyType{}
//			s.Require().NoError(bson.Unmarshal(bytes, &rtStruct))
//			s.Assert().Equal(
//				myThing,
//				rtStruct,
//			)
//		},
//	)
//}
//
//func (s *mySuite) Test_Option_JSON() {
//	type MyType struct {
//		IsNone  Option[int]
//		Omitted Option[int]
//		IsSome  Option[bool]
//	}
//
//	type MyTypePtrs struct {
//		IsNone  *int
//		Omitted *int
//		IsSome  *bool
//	}
//
//	s.Run(
//		"marshal pointer, unmarshal Option",
//		func() {
//
//			bytes, err := json.Marshal(MyTypePtrs{
//				IsNone: pointerTo(234),
//				IsSome: pointerTo(false),
//			})
//			s.Require().NoError(err)
//
//			rt := MyType{}
//			s.Require().NoError(json.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				MyType{
//					IsNone: Some(234),
//					IsSome: Some(false),
//				},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"marshal Option, unmarshal pointer",
//		func() {
//
//			bytes, err := json.Marshal(MyType{
//				IsNone: Some(234),
//				IsSome: Some(false),
//			})
//			s.Require().NoError(err)
//
//			rt := MyTypePtrs{}
//			s.Require().NoError(json.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				MyTypePtrs{
//					IsNone: pointerTo(234),
//					IsSome: pointerTo(false),
//				},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"round-trip bson.D",
//		func() {
//			simpleDoc := bson.D{
//				{"a", None[int]()},
//				{"b", Some(123)},
//			}
//
//			bytes, err := json.Marshal(simpleDoc)
//			s.Require().NoError(err)
//
//			rt := bson.D{}
//			s.Require().NoError(json.Unmarshal(bytes, &rt))
//
//			s.Assert().Equal(
//				bson.D{{"a", nil}, {"b", float64(123)}},
//				rt,
//			)
//		},
//	)
//
//	s.Run(
//		"round-trip struct",
//		func() {
//			myThing := MyType{None[int](), None[int](), Some(true)}
//
//			bytes, err := json.Marshal(&myThing)
//			s.Require().NoError(err)
//
//			rtStruct := MyType{}
//			s.Require().NoError(json.Unmarshal(bytes, &rtStruct))
//			s.Assert().Equal(
//				myThing,
//				rtStruct,
//			)
//		},
//	)
//}
//
//func (s *mySuite) Test_Option_NoNilSome() {
//	assertPanics(s, (chan int)(nil))
//	assertPanics(s, (func())(nil))
//	assertPanics(s, any(nil))
//	assertPanics(s, map[int]any(nil))
//	assertPanics(s, []any(nil))
//	assertPanics(s, (*any)(nil))
//}
//
//func (s *mySuite) Test_Option_Pointer() {
//	opt := Some(123)
//	ptr := opt.ToPointer()
//	*ptr = 1234
//
//	s.Assert().Equal(
//		Some(123),
//		opt,
//		"ToPointer() sholuldn't let caller alter Option value",
//	)
//
//	opt2 := FromPointer(ptr)
//	*ptr = 2345
//	s.Assert().Equal(
//		Some(1234),
//		opt2,
//		"FromPointer() sholuldn't let caller alter Option value",
//	)
//}
//
//func (s *mySuite) Test_Option() {
//
//	//nolint:testifylint  // None is, in fact, the expected value.
//	s.Assert().Equal(
//		None[int](),
//		Option[int]{},
//		"zero value is None",
//	)
//
//	//nolint:testifylint
//	s.Assert().Equal(Some(1), Some(1), "same internal value")
//	s.Assert().NotEqual(Some(1), Some(2), "different internal value")
//
//	foo := "foo"
//	fooPtr := Some(foo).ToPointer()
//
//	s.Assert().Equal(&foo, fooPtr)
//
//	s.Assert().Equal(Some(foo), FromPointer(fooPtr))
//
//	s.Assert().Equal(
//		foo,
//		Some(foo).OrZero(),
//	)
//
//	s.Assert().Empty(
//		None[string]().OrZero(),
//	)
//
//	s.Assert().Equal(
//		"elf",
//		None[string]().OrElse("elf"),
//	)
//
//	val, has := Some(123).Get()
//	s.Assert().True(has)
//	s.Assert().Equal(123, val)
//
//	val, has = None[int]().Get()
//	s.Assert().False(has)
//	s.Assert().Equal(0, val)
//
//	some := Some(456)
//	s.Assert().True(some.IsSome())
//	s.Assert().False(some.IsNone())
//
//	none := None[int]()
//	s.Assert().False(none.IsSome())
//	s.Assert().True(none.IsNone())
//}
//
//func (s *mySuite) Test_Option_IfNonZero() {
//	assertIfNonZero(s, 0, 1)
//	assertIfNonZero(s, "", "a")
//	assertIfNonZero(s, []int(nil), []int{})
//	assertIfNonZero(s, map[int]int(nil), map[int]int{})
//	assertIfNonZero(s, any(nil), any(0))
//	assertIfNonZero(s, bson.D(nil), bson.D{})
//
//	type myStruct struct {
//		//nolint:unused // We need a field in this struct to distinguish it from its empty value.
//		name string
//	}
//
//	assertIfNonZero(s, myStruct{}, myStruct{"foo"})
//}
//
//func assertIfNonZero[T any](s *mySuite, zeroVal, nonZeroVal T) {
//	noneOpt := IfNotZero(zeroVal)
//	someOpt := IfNotZero(nonZeroVal)
//
//	s.Assert().Equal(None[T](), noneOpt)
//	s.Assert().Equal(Some(nonZeroVal), someOpt)
//}
//
//func pointerTo[T any](val T) *T {
//	return &val
//}
//
//func assertPanics[T any](s *mySuite, val T) {
//	s.T().Helper()
//
//	s.Assert().Panics(
//		func() { Some(val) },
//		"Some(%T)",
//		val,
//	)
//
//	s.Assert().Panics(
//		func() { FromPointer(&val) },
//		"FromPointer(&%T)",
//		val,
//	)
//}
