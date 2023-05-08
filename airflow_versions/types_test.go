package airflowversions

func (s *Suite) TestNewAirflowVersion() {
	av, err := NewAirflowVersion("2.0.0-1", []string{"2.0.0-1-buster-onbuild"})
	s.NoError(err)
	s.Equal(uint64(2), av.Major())
	s.Equal(uint64(0), av.Minor())
	s.Equal(uint64(0), av.Patch())
	s.Equal(uint64(1), av.postN1)
}

func (s *Suite) TestNewAirflowVersionError() {
	av, err := NewAirflowVersion("-1", []string{"2.0.0-1-buster-onbuild"})
	s.Error(err)
	s.Nil(av)
}

func (s *Suite) TestNewAirflowVersionWithoutPostN1() {
	av, err := NewAirflowVersion("2.0.0", []string{"2.0.0-buster-onbuild"})
	s.NoError(err)
	s.Equal(uint64(2), av.Major())
	s.Equal(uint64(0), av.Minor())
	s.Equal(uint64(0), av.Patch())
	s.Equal(uint64(0), av.postN1)
}

func (s *Suite) TestNewAirflowVersionWithPostN1() {
	av, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(uint64(1), av.Major())
	s.Equal(uint64(10), av.Minor())
	s.Equal(uint64(5), av.Patch())
	s.Equal(uint64(11), av.postN1)
}

func (s *Suite) TestCompareAirflowVersionsHighPostN1() {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(1, av1.Compare(av2))
}

func (s *Suite) TestCompareAirflowVersionsLowPostN1() {
	av1, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(-1, av1.Compare(av2))
}

func (s *Suite) TestCompareAirflowVersionsSamePostN1() {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(0, av1.Compare(av2))
}

func (s *Suite) TestCompareAirflowVersionsMajor() {
	av1, err := NewAirflowVersion("2.10.5-11", []string{"2.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(1, av1.Compare(av2))
}

func (s *Suite) TestCompareAirflowVersionsMinor() {
	av1, err := NewAirflowVersion("1.11.5-11", []string{"1.11.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(1, av1.Compare(av2))
}

func (s *Suite) TestCompareAirflowVersionsPatch() {
	av1, err := NewAirflowVersion("1.10.6", []string{"1.11.6-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(1, av1.Compare(av2))
}

func (s *Suite) TestGreaterThan() {
	av1, err := NewAirflowVersion("1.10.5-11", []string{"1.10.5-11-alpine3.10-onbuild"})
	s.NoError(err)
	av2, err := NewAirflowVersion("1.10.5", []string{"1.10.5-alpine3.10-onbuild"})
	s.NoError(err)
	s.Equal(true, av1.GreaterThan(av2))
}

func (s *Suite) Test_compareSegment() {
	s.Equal(0, compareSegment(1, 1))
	s.Equal(-1, compareSegment(1, 2))
	s.Equal(1, compareSegment(2, 1))
}
